package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Xara-AI/sitedex/internal/search"
)

type searchRequest struct {
	Site  string `json:"site"`
	Query string `json:"query"`
	Limit int    `json:"limit"`
	Fresh bool   `json:"fresh"`
	Type  string `json:"type"`

	// Soft opts into suffix-relaxed (prefix) matching as a fallback when
	// the exact query matches nothing — see search.Request.Soft. Off by
	// default: exact matching is the fast, predictable path.
	Soft bool `json:"soft"`
}

// searchResult mirrors search.Result (== index.Result) as wire JSON,
// matching the field set CLAUDE.md documents for POST /v1/search —
// nothing more. Fields that don't apply to a given result (e.g. price on
// a page-type result) are omitted rather than sent as zero values.
type searchResult struct {
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Type         string   `json:"type"`
	Price        *float64 `json:"price,omitempty"`
	Currency     string   `json:"currency,omitempty"`
	Availability string   `json:"availability,omitempty"`
	Image        string   `json:"image,omitempty"`
	Snippet      string   `json:"snippet,omitempty"`
	Score        float64  `json:"score"`
	VerifiedAt   *string  `json:"verified_at,omitempty"` // present only when fresh-verified
}

type searchResponse struct {
	Results []searchResult `json:"results"`
	Source  string         `json:"source"`
	TookMs  int64          `json:"took_ms"`
}

// shouldAutoIndex reports whether a search response should trigger a
// background full crawl, per config.SearchConfig.AutoIndexOnColdQuery
// ("auto_index_on_cold_query" — see CLAUDE.md's cold-path description).
func shouldAutoIndex(source string, enabled bool) bool {
	return enabled && source == "site-search"
}

func toSearchResult(r search.Result) searchResult {
	out := searchResult{
		Title: r.Title, URL: r.URL, Type: r.Type, Currency: r.Currency,
		Availability: r.Availability, Image: r.Image, Snippet: r.Snippet, Score: r.Score,
	}
	if r.HasPrice {
		price := r.Price
		out.Price = &price
	}
	if r.Verified {
		ts := r.VerifiedAt.UTC().Format(time.RFC3339)
		out.VerifiedAt = &ts
	}
	return out
}

// handleSearch serves POST /v1/search. The search itself runs on
// r.Context() (not the server's overall lifetime context), so a client
// disconnect or the graceful-shutdown grace period bounds it naturally.
// serveCtx is only needed for the auto-index-on-cold-query side effect
// below, which — like a crawl job — outlives this request.
func (s *Server) handleSearch(serveCtx context.Context, w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Site == "" || req.Query == "" {
		writeError(w, http.StatusBadRequest, "site and query are required")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	resp, err := s.searcher.SearchFresh(r.Context(), search.Request{
		Site: req.Site, Query: req.Query, Limit: limit, Type: req.Type, Fresh: req.Fresh, Soft: req.Soft,
		FreshTopN:    s.cfg.Search.FreshTopN,
		FreshTimeout: time.Duration(s.cfg.Search.FreshTimeoutMS) * time.Millisecond,
	})
	took := time.Since(start)
	s.metrics.observeSearch(took, err != nil)
	if err != nil {
		s.logger.Error("search failed", "site", req.Site, "query", req.Query, "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Per CLAUDE.md's cold-path description: a site-search response means
	// this site wasn't indexed, so — if enabled — kick off a background
	// full crawl now, fire-and-forget, so the next query on this site is
	// warm. This doesn't dedupe concurrent cold queries for the same site
	// into one crawl job; a burst of queries for a newly-seen site can
	// enqueue more than one redundant crawl — an accepted v1 simplification.
	if shouldAutoIndex(resp.Source, s.cfg.Search.AutoIndexOnColdQuery) {
		j := s.jobs.create("https://" + req.Site + "/")
		s.logger.Info("search: auto-indexing after cold query", "site", req.Site, "job_id", j.id)
		go s.runCrawlJob(serveCtx, j)
	}

	results := make([]searchResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = toSearchResult(r)
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results, Source: resp.Source, TookMs: took.Milliseconds()})
}
