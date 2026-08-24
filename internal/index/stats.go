package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Stats summarizes one site's index, for `sitedex sites`.
type Stats struct {
	PageCount     int
	ChunkCount    int
	ProductCount  int
	LastCrawledAt string // RFC3339, empty if the site has no pages yet
}

// Stats reports summary counts for this index.
func (db *DB) Stats() (Stats, error) {
	var s Stats
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&s.PageCount); err != nil {
		return s, fmt.Errorf("count pages: %w", err)
	}
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&s.ChunkCount); err != nil {
		return s, fmt.Errorf("count chunks: %w", err)
	}
	if err := db.sql.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&s.ProductCount); err != nil {
		return s, fmt.Errorf("count products: %w", err)
	}
	var last sql.NullString
	if err := db.sql.QueryRow(`SELECT MAX(crawled_at) FROM pages`).Scan(&last); err != nil {
		return s, fmt.Errorf("max crawled_at: %w", err)
	}
	if last.Valid {
		s.LastCrawledAt = last.String
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
