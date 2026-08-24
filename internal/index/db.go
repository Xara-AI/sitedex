// Package index manages the per-site SQLite FTS5 index (pages, chunks,
// products, and their FTS mirrors), keyed by URL, with the unicode61
// tokenizer and remove_diacritics=2 so Romanian diacritics match
// unaccented queries.
//
// See CLAUDE.md, "Index".
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DB is one site's SQLite index, at <data_dir>/<site>/index.db.
type DB struct {
	sql *sql.DB
}

// Open opens (creating and migrating if needed) the index for site under
// dataDir.
func Open(dataDir, site string) (*DB, error) {
	dir := filepath.Join(dataDir, site)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return open(filepath.Join(dir, "index.db"))
}

// Path returns the on-disk path of a site's index.db, without opening it —
// used by callers that just need to check for existence (e.g. `sitedex
// sites`).
func Path(dataDir, site string) string {
	return filepath.Join(dataDir, site, "index.db")
}

func open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}
	// SQLite allows only one writer at a time; keeping a single connection
	// avoids SQLITE_BUSY errors from this process's own concurrent use
	// rather than papering over them with retries.
	sqlDB.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}

	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate index db: %w", err)
	}
	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.sql.Close()
}
