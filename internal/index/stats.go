package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Stats summarizes one site's index, for `sitedex sites` and GET /v1/sites.
type Stats struct {
	PageCount       int
	ChunkCount      int
	ProductCount    int
	LastCrawledAt   string // RFC3339, empty if the site has no pages yet
	OldestCrawledAt string // RFC3339, empty if the site has no pages yet
	LastVerifiedAt  string // RFC3339, empty if nothing has ever been fresh-verified

	// ExtractionMethods counts products by how they were extracted
	// (json-ld, microdata, opengraph, heuristic, llm) — a quick signal of
	// how reliable this site's product data is, without pulling the full
	// per-item listing from ListItems.
	ExtractionMethods map[string]int
}

// Stats reports summary counts for this index.
func (db *DB) Stats() (Stats, error) {
	s := Stats{ExtractionMethods: map[string]int{}}
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&s.PageCount); err != nil {
		return s, fmt.Errorf("count pages: %w", err)
	}
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&s.ChunkCount); err != nil {
		return s, fmt.Errorf("count chunks: %w", err)
	}
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&s.ProductCount); err != nil {
		return s, fmt.Errorf("count products: %w", err)
	}

	var last, oldest sql.NullString
	if err := db.sql.QueryRow(`SELECT MAX(crawled_at), MIN(crawled_at) FROM pages`).Scan(&last, &oldest); err != nil {
		return s, fmt.Errorf("min/max crawled_at: %w", err)
	}
	if last.Valid {
		s.LastCrawledAt = last.String
	}
	if oldest.Valid {
		s.OldestCrawledAt = oldest.String
	}

	var lastVerified sql.NullString
	if err := db.sql.QueryRow(`SELECT MAX(verified_at) FROM products WHERE verified_at != ''`).Scan(&lastVerified); err != nil {
		return s, fmt.Errorf("max verified_at: %w", err)
	}
	if lastVerified.Valid {
		s.LastVerifiedAt = lastVerified.String
	}

	rows, err := db.sql.Query(`SELECT extraction_method, COUNT(*) FROM products GROUP BY extraction_method`)
	if err != nil {
		return s, fmt.Errorf("extraction method breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var method string
		var count int
		if err := rows.Scan(&method, &count); err != nil {
			return s, fmt.Errorf("scan extraction method row: %w", err)
		}
		s.ExtractionMethods[method] = count
	}
	if err := rows.Err(); err != nil {
		return s, fmt.Errorf("extraction method rows: %w", err)
	}

	return s, nil
}

// ListSites returns the site directory names under dataDir that have an
// index.db, sorted alphabetically.
func ListSites(dataDir string) ([]string, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read data dir: %w", err)
	}

	var sites []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dataDir, e.Name(), "index.db")); err == nil {
			sites = append(sites, e.Name())
		}
	}
	sort.Strings(sites)
	return sites, nil
}
