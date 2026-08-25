package server

import (
	"net/http"

	"github.com/Xara-AI/sitedex/internal/index"
)

type siteDTO struct {
	Site              string         `json:"site"`
	Pages             int            `json:"pages"`
	Chunks            int            `json:"chunks"`
	Products          int            `json:"products"`
	LastCrawledAt     string         `json:"last_crawled_at,omitempty"`
	OldestCrawledAt   string         `json:"oldest_crawled_at,omitempty"`
	LastVerifiedAt    string         `json:"last_verified_at,omitempty"`
	ExtractionMethods map[string]int `json:"extraction_methods,omitempty"`
}

// handleSites serves GET /v1/sites.
func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	sites, err := index.ListSites(s.cfg.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sites failed")
		return
	}

	out := make([]siteDTO, 0, len(sites))
	for _, site := range sites {
		stats, err := s.siteStats(site)
		if err != nil {
			s.logger.Warn("sites: stats failed", "site", site, "err", err)
			continue
		}
		out = append(out, siteDTO{
			Site: site, Pages: stats.PageCount, Chunks: stats.ChunkCount,
			Products: stats.ProductCount, LastCrawledAt: stats.LastCrawledAt,
			OldestCrawledAt: stats.OldestCrawledAt, LastVerifiedAt: stats.LastVerifiedAt,
			ExtractionMethods: stats.ExtractionMethods,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": out})
}

// siteStats opens (briefly) and reads one site's index stats. Sites are
// typically few and this endpoint is infrequently polled, so a fresh
// open/close per call is simpler than maintaining a connection cache —
// same tradeoff /metrics makes.
func (s *Server) siteStats(site string) (index.Stats, error) {
	idx, err := index.Open(s.cfg.DataDir, site)
	if err != nil {
		return index.Stats{}, err
	}
	defer func() { _ = idx.Close() }()
	return idx.Stats()
}

// handleHealthz serves GET /healthz.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
