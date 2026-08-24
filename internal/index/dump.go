package index

import "fmt"

// ChunkDump is one chunk plus its page's metadata, denormalized for
// full-corpus dumps (e.g. JSONL export).
type ChunkDump struct {
	PageURL     string
	Title       string
	Description string
	Lang        string
	Ordinal     int
	HeadingPath string
	Text        string
}

// AllChunks returns every chunk in the index, ordered by page URL then
// ordinal — the full corpus, for JSONL export.
func (db *DB) AllChunks() ([]ChunkDump, error) {
	rows, err := db.sql.Query(`
		SELECT c.page_url, p.title, p.description, p.lang, c.ord, c.heading_path, c.text
		FROM chunks c
		JOIN pages p ON p.url = c.page_url
		ORDER BY c.page_url, c.ord
	`)
	if err != nil {
		return nil, fmt.Errorf("query all chunks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ChunkDump
	for rows.Next() {
		var d ChunkDump
		if err := rows.Scan(&d.PageURL, &d.Title, &d.Description, &d.Lang, &d.Ordinal, &d.HeadingPath, &d.Text); err != nil {
			return nil, fmt.Errorf("scan chunk dump: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
