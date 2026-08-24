package index

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
`

func (db *DB) migrate() error {
	_, err := db.sql.Exec(schema)
	return err
}
