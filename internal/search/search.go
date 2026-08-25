// Package search turns a query into ranked results via three paths: warm
// (FTS5 BM25 over the local index, this file), fresh (concurrent live
// re-fetch and re-verification of top results within a response time
// budget, fresh.go), and cold (platform search-endpoint detection for a
// not-yet-indexed site, with background auto-indexing).
//
// Target milestones: M3 (warm path), M5 (fresh path), M6 (cold path — not
// implemented yet). See CLAUDE.md, "Search".
package search

import (
	"fmt"
	"os"

	"github.com/Xara-AI/sitedex/internal/index"
)

// Result is one page-level search hit.
type Result = index.Result

// Searcher runs searches against sites indexed under dataDir. userAgent
// identifies this process to sites it re-fetches during fresh-verify.
type Searcher struct {
	dataDir   string
	userAgent string
}

// New builds a Searcher rooted at dataDir (config.Config.DataDir).
func New(dataDir, userAgent string) *Searcher {
	return &Searcher{dataDir: dataDir, userAgent: userAgent}
}

// Search runs a warm-path (index-only) search for site, returning up to
// limit results ordered best-first. It returns a clear error if site has
// never been crawled, rather than silently creating an empty index. This
// is the CLI's entry point; the HTTP API uses SearchFresh instead (see
// fresh.go), which treats an uncrawled site as zero results rather than an
// error — appropriate for a network API where "never crawled yet" is a
// normal, expected state pending M6's cold-path fallback.
func (s *Searcher) Search(site, query string, limit int) ([]Result, error) {
	idx, err := s.openSite(site)
	if err != nil {
		return nil, err
	}
	defer func() { _ = idx.Close() }()

	results, err := idx.Search(query, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return results, nil
}

// SearchSoft is Search with the suffix-relaxation fallback opted in (see
// index.DB.SearchSoft): only takes effect when the strict query would
// otherwise come back empty.
func (s *Searcher) SearchSoft(site, query string, limit int) ([]Result, error) {
	idx, err := s.openSite(site)
	if err != nil {
		return nil, err
	}
	defer func() { _ = idx.Close() }()

	results, err := idx.SearchSoft(query, limit, "")
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return results, nil
}

// openSite opens site's index, erroring clearly if it's never been
// crawled rather than silently creating an empty index.
func (s *Searcher) openSite(site string) (*index.DB, error) {
	dbPath := index.Path(s.dataDir, site)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("no index found for %q (looked in %s) — run `sitedex crawl --site <url>` first", site, dbPath)
	}
	idx, err := index.Open(s.dataDir, site)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	return idx, nil
}
