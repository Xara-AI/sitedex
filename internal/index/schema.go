package index

import "fmt"

// schema is applied on every Open (all statements are idempotent), so
// opening an existing index.db is just as safe as creating a new one.
//
// Design notes (see CLAUDE.md, "Index"):
//   - pages/chunks/products are plain tables; chunks_fts/products_fts are
//     separate FTS5 "mirror" tables kept in sync from Go, not SQLite
//     triggers or FTS5 external-content mode. That keeps the write path
//     explicit and easy to reason about/test, at the cost of a little
//     duplicated data.
//   - Both FTS5 tables use "unicode61 remove_diacritics 2" so a query for
//     "pantofi" matches "pantofi", "PANTOFI", and diacritic variants
//     (Romanian text in particular).
//   - products/products_fts exist now so M4's product extraction doesn't
//     need a schema migration; nothing in M3 writes to them.
const schema = `
CREATE TABLE IF NOT EXISTS pages (
    url           TEXT PRIMARY KEY,
    title         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    lang          TEXT NOT NULL DEFAULT '',
    hash          TEXT NOT NULL DEFAULT '',
    crawled_at    TEXT NOT NULL DEFAULT '',
    etag          TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS chunks (
    page_url     TEXT NOT NULL REFERENCES pages(url) ON DELETE CASCADE,
    ord          INTEGER NOT NULL,
    heading_path TEXT NOT NULL DEFAULT '',
    text         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (page_url, ord)
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    page_url UNINDEXED,
    ord UNINDEXED,
    title,
    heading_path,
    text,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS products (
    page_url           TEXT PRIMARY KEY REFERENCES pages(url) ON DELETE CASCADE,
    name               TEXT NOT NULL DEFAULT '',
    description        TEXT NOT NULL DEFAULT '',
    price              REAL,
    currency           TEXT NOT NULL DEFAULT '',
    availability       TEXT NOT NULL DEFAULT '',
    image              TEXT NOT NULL DEFAULT '',
    extraction_method  TEXT NOT NULL DEFAULT '',
    raw_json           TEXT NOT NULL DEFAULT ''
);

CREATE VIRTUAL TABLE IF NOT EXISTS products_fts USING fts5(
    page_url UNINDEXED,
    name,
    description,
    tokenize = 'unicode61 remove_diacritics 2'
);

-- seq_counter hands out the monotonically increasing "seq" stamped onto
-- pages/products rows on every write (see nextSeq in seq.go). It backs the
-- GET /v1/sites/{site}/items changefeed cursor: a poller passes back the
-- highest seq it's seen and gets only rows touched since. A dedicated
-- counter (rather than, say, MAX(seq)+1 read at write time) keeps seq
-- assignment correct even though pages and products share one sequence
-- space across two tables.
CREATE TABLE IF NOT EXISTS seq_counter (
    id    INTEGER PRIMARY KEY CHECK (id = 1),
    value INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO seq_counter (id, value) VALUES (1, 0);
`

// columnAdditions are ALTER TABLE ADD COLUMN statements for columns
// introduced after a table's initial CREATE TABLE IF NOT EXISTS — those
// don't retrofit existing databases (this one included: the "seq" and
// "verified_at" columns below were added post-v0.1.0), so unlike the rest
// of schema they're applied via ensureColumn, which no-ops when the column
// is already present.
var columnAdditions = []struct{ table, column, def string }{
	{"pages", "seq", "INTEGER NOT NULL DEFAULT 0"},
	{"products", "seq", "INTEGER NOT NULL DEFAULT 0"},
	{"products", "verified_at", "TEXT NOT NULL DEFAULT ''"},
}

func (db *DB) migrate() error {
	if _, err := db.sql.Exec(schema); err != nil {
		return err
	}
	for _, c := range columnAdditions {
		added, err := db.ensureColumn(c.table, c.column, c.def)
		if err != nil {
			return fmt.Errorf("ensure column %s.%s: %w", c.table, c.column, err)
		}
		// Rows written before this column existed get seq=0 from the ALTER
		// TABLE's DEFAULT — indistinguishable from "not yet seen" as far as
		// ListItems' `seq > since_seq` filter is concerned, which would
		// otherwise hide every pre-upgrade row from GET
		// /v1/sites/{site}/items permanently (since_seq=0, the documented
		// "everything" bootstrap value, filters on "> 0"). Backfilling real
		// seq values here — once, only for rows the ALTER just left at
		// zero — is what makes existing indexes show up without requiring
		// a re-crawl after upgrading to the version that added seq.
		if added && c.column == "seq" {
			if err := db.backfillSeq(c.table); err != nil {
				return fmt.Errorf("backfill seq for %s: %w", c.table, err)
			}
		}
	}
	return nil
}

// ensureColumn adds column to table if it isn't already there, reporting
// whether it actually did so. SQLite has no "ALTER TABLE ... ADD COLUMN IF
// NOT EXISTS", so this checks PRAGMA table_info first rather than relying
// on error-string matching.
func (db *DB) ensureColumn(table, column, def string) (added bool, err error) {
	rows, err := db.sql.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid       int
			name, typ string
			notNull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return false, nil // already present
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	if _, err := db.sql.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def)); err != nil {
		return false, err
	}
	return true, nil
}

// seqBackfillKeyColumn is each seq-bearing table's primary key, used to
// order and target backfillSeq's UPDATEs.
var seqBackfillKeyColumn = map[string]string{
	"pages":    "url",
	"products": "page_url",
}

// backfillSeq assigns a real, monotonically increasing seq (via nextSeq,
// the same counter live writes use) to every row in table currently stuck
// at the ALTER TABLE default of 0 — see the comment in migrate. One
// row-per-transaction rather than a single bulk statement: this only runs
// once per table, the first time its index.db is opened under a binary new
// enough to have the seq column, so simplicity wins over a cleverer
// window-function UPDATE.
func (db *DB) backfillSeq(table string) error {
	keyColumn, ok := seqBackfillKeyColumn[table]
	if !ok {
		return fmt.Errorf("backfillSeq: no key column registered for table %q", table)
	}

	rows, err := db.sql.Query(fmt.Sprintf(`SELECT %s FROM %s WHERE seq = 0 ORDER BY %s`, keyColumn, table, keyColumn))
	if err != nil {
		return fmt.Errorf("query rows to backfill: %w", err)
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan backfill key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("backfill rows: %w", err)
	}
	_ = rows.Close()

	for _, k := range keys {
		tx, err := db.sql.Begin()
		if err != nil {
			return fmt.Errorf("begin backfill tx: %w", err)
		}
		seq, err := nextSeq(tx)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET seq = ? WHERE %s = ?`, table, keyColumn), seq, k); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("backfill update: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit backfill tx: %w", err)
		}
	}
	return nil
}
