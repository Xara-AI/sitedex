package index

import (
	"database/sql"
	"fmt"
	"time"
)

// PageRecord is one page's metadata, as stored in the pages table.
type PageRecord struct {
	URL          string
	Title        string
	Description  string
	Lang         string
	Hash         string
	CrawledAt    time.Time
	ETag         string
	LastModified string
}

// ChunkRecord is one heading-anchored chunk of a page's markdown, as
// stored in chunks/chunks_fts.
type ChunkRecord struct {
	Ordinal     int
	HeadingPath string
	Text        string
}

// IndexPage upserts page's metadata and replaces all of its chunks
// (delete-then-insert, so a page that shrank doesn't leave orphaned
// trailing chunks behind), inside a single transaction.
func (db *DB) IndexPage(page PageRecord, chunks []ChunkRecord) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if already committed

	seq, err := nextSeq(tx)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO pages (url, title, description, lang, hash, crawled_at, etag, last_modified, seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			description = excluded.description,
			lang = excluded.lang,
			hash = excluded.hash,
			crawled_at = excluded.crawled_at,
			etag = excluded.etag,
			last_modified = excluded.last_modified,
			seq = excluded.seq
	`, page.URL, page.Title, page.Description, page.Lang, page.Hash,
		page.CrawledAt.UTC().Format(time.RFC3339), page.ETag, page.LastModified, seq); err != nil {
		return fmt.Errorf("upsert page: %w", err)
	}

	if err := deletePageChunks(tx, page.URL); err != nil {
		return err
	}

	for _, c := range chunks {
		if _, err := tx.Exec(`INSERT INTO chunks (page_url, ord, heading_path, text) VALUES (?, ?, ?, ?)`,
			page.URL, c.Ordinal, c.HeadingPath, c.Text); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO chunks_fts (page_url, ord, title, heading_path, text) VALUES (?, ?, ?, ?, ?)`,
			page.URL, c.Ordinal, page.Title, c.HeadingPath, c.Text); err != nil {
			return fmt.Errorf("insert chunk fts: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// DeletePage removes a page, its chunks, and (if present) its product
// record, along with their FTS mirror rows.
func (db *DB) DeletePage(pageURL string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deletePageChunks(tx, pageURL); err != nil {
		return err
	}
	if err := deleteProductRows(tx, pageURL); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pages WHERE url = ?`, pageURL); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}

	return tx.Commit()
}

func deletePageChunks(tx *sql.Tx, pageURL string) error {
	if _, err := tx.Exec(`DELETE FROM chunks WHERE page_url = ?`, pageURL); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM chunks_fts WHERE page_url = ?`, pageURL); err != nil {
		return fmt.Errorf("delete chunks fts: %w", err)
	}
	return nil
}
