// Package search turns a query into ranked results via three paths: warm
// (FTS5 BM25 over the local index — implemented here), fresh (concurrent
// live re-fetch and re-verification of top results within a response time
// budget), and cold (platform search-endpoint detection for a
// not-yet-indexed site, with background auto-indexing).
//
// Target milestones: M3 (warm path, this file), M5 (fresh path), M6 (cold
// path). See CLAUDE.md, "Search".
package search

import (
	"fmt"
	"os"

	"github.com/Xara-AI/sitedex/internal/index"
)

// Result is one page-level search hit.
type Result = index.Result

// Searcher runs searches against sites indexed under dataDir.
type Searcher struct {
	dataDir string
}

// New builds a Searcher rooted at dataDir (config.Config.DataDir).
func New(dataDir string) *Searcher {
	return &Searcher{dataDir: dataDir}
}

// Search runs a warm-path (index-only) search for site, returning up to
// limit results ordered best-first. It returns a clear error if site has
// never been crawled, rather than silently creating an empty index.
func (s *Searcher) Search(site, query string, limit int) ([]Result, error) {
	dbPath := index.Path(s.dataDir, site)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("no index found for %q (looked in %s) — run `sitedex crawl --site <url>` first", site, dbPath)
	}

	idx, err := index.Open(s.dataDir, site)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	results, err := idx.Search(query, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return results, nil
}
