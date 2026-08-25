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
		if err := db.ensureColumn(c.table, c.column, c.def); err != nil {
			return fmt.Errorf("ensure column %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// ensureColumn adds column to table if it isn't already there. SQLite has
// no "ALTER TABLE ... ADD COLUMN IF NOT EXISTS", so this checks
// PRAGMA table_info first rather than relying on error-string matching.
func (db *DB) ensureColumn(table, column, def string) error {
	rows, err := db.sql.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
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
			return err
		}
		if name == column {
			return nil // already present
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.sql.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def))
	return err
}
