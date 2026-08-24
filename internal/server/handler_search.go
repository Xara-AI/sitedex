package server

import (
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

// handleSearch serves POST /v1/search. It uses r.Context() (not the
// server's overall lifetime context) so a client disconnect or a
// graceful-shutdown grace period bounds it naturally — unlike a crawl
// job, nothing here needs to outlive the request.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
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
		Site: req.Site, Query: req.Query, Limit: limit, Type: req.Type, Fresh: req.Fresh,
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

	results := make([]searchResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = toSearchResult(r)
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results, Source: resp.Source, TookMs: took.Milliseconds()})
}
