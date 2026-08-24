package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/index"
)

type crawlTriggerRequest struct {
	Site string `json:"site"`
}

// handleCrawlTrigger serves POST /v1/crawl: it enqueues a background
// crawl job and returns immediately with a job_id to poll. serveCtx is
// the server's overall lifetime context (see routes) — the crawl runs in
// a goroutine that outlives this request, so it needs a context tied to
// the server's shutdown, not the request's.
func (s *Server) handleCrawlTrigger(serveCtx context.Context, w http.ResponseWriter, r *http.Request) {
	var req crawlTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Site == "" {
		writeError(w, http.StatusBadRequest, "site is required")
		return
	}

	j := s.jobs.create(req.Site)
	go s.runCrawlJob(serveCtx, j)

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": j.id})
}

// handleCrawlStatus serves GET /v1/crawl/{job}.
func (s *Server) handleCrawlStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("job")
	j, ok := s.jobs.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, http.StatusOK, j.toDTO())
}

// runCrawlJob runs one crawl to completion, updating the job tracker and
// metrics as it goes. It's also how the background re-crawl scheduler
// (scheduler.go) triggers a crawl — same code path as an API-triggered one.
func (s *Server) runCrawlJob(ctx context.Context, j *job) {
	s.jobs.setRunning(j.id)

	site, err := crawler.SiteForURL(j.site)
	if err != nil {
		s.jobs.setFailed(j.id, err)
		s.metrics.crawlsFailed.Add(1)
		return
	}

	idx, err := index.Open(s.cfg.DataDir, site)
	if err != nil {
		s.jobs.setFailed(j.id, err)
		s.metrics.crawlsFailed.Add(1)
		s.logger.Error("crawl job: open index failed", "job_id", j.id, "site", j.site, "err", err)
		return
	}
	defer func() { _ = idx.Close() }()

	c := crawler.New(s.cfg.Crawl, s.cfg.Chunking, s.cfg.LLMExtractor, s.cfg.DataDir, exportWriter{}, indexAdapter{idx},
		func(format string, a ...any) { s.logger.Debug("crawl", "msg", fmt.Sprintf(format, a...)) })

	res, err := c.Crawl(ctx, j.site)
	if err != nil {
		s.jobs.setFailed(j.id, err)
		s.metrics.crawlsFailed.Add(1)
		s.logger.Error("crawl job failed", "job_id", j.id, "site", j.site, "err", err)
		return
	}

	s.jobs.setSucceeded(j.id, res)
	s.metrics.crawlsSucceeded.Add(1)
	s.logger.Info("crawl job succeeded", "job_id", j.id, "site", res.Site,
		"fetched", res.PagesFetched, "skipped", res.PagesSkipped, "failed", res.PagesFailed)
}
