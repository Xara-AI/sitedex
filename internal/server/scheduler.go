package server

import (
	"context"
	"time"

	"github.com/Xara-AI/sitedex/internal/index"
)

// schedulerCheckInterval is how often the scheduler wakes up to check
// whether any site is due for a re-crawl. It's independent of (and much
// finer-grained than) crawl.recrawl_interval itself, which defaults to
// 24h — checking every 5 minutes keeps the actual re-crawl trigger within
// a few minutes of its due time without meaningful overhead.
const schedulerCheckInterval = 5 * time.Minute

// startScheduler runs the background re-crawl loop until ctx is
// canceled, per "serve is ... HTTP API over all indexed sites, background
// re-crawls on schedule" (CLAUDE.md, Command Surface). It returns a stop
// function that blocks until the loop has actually exited, for orderly
// shutdown sequencing.
func (s *Server) startScheduler(ctx context.Context) (stop func()) {
	if time.Duration(s.cfg.Crawl.RecrawlInterval) <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(schedulerCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recrawlStaleSites(ctx)
			}
		}
	}()
	return func() { <-done }
}

// recrawlStaleSites enqueues a crawl job (the same path POST /v1/crawl
// uses) for every indexed site whose last crawl is older than
// crawl.recrawl_interval.
//
// It re-crawls from "https://<site>/" rather than the original seed URL
// (which isn't retained anywhere once a crawl completes) — sitemap.xml
// and link discovery from root should reconverge on the same page set for
// the overwhelming majority of sites. A site that was only ever crawled
// from a non-root path, or over plain http://, is a known limitation of
// this v1 approximation.
func (s *Server) recrawlStaleSites(ctx context.Context) {
	sites, err := index.ListSites(s.cfg.DataDir)
	if err != nil {
		s.logger.Warn("scheduler: list sites failed", "err", err)
		return
	}

	interval := time.Duration(s.cfg.Crawl.RecrawlInterval)
	for _, site := range sites {
		stats, err := s.siteStats(site)
		if err != nil {
			s.logger.Warn("scheduler: stats failed", "site", site, "err", err)
			continue
		}
		if !isStale(stats.LastCrawledAt, interval) {
			continue
		}

		seedURL := "https://" + site + "/"
		j := s.jobs.create(seedURL)
		s.logger.Info("scheduler: triggering re-crawl", "site", site, "last_crawled_at", stats.LastCrawledAt, "job_id", j.id)
		go s.runCrawlJob(ctx, j)
	}
}

// isStale reports whether lastCrawledAt (RFC3339, possibly empty for a
// site with no pages yet) is older than interval. An empty or unparsable
// timestamp is treated as stale, so a partially-indexed site still gets
// picked up.
func isStale(lastCrawledAt string, interval time.Duration) bool {
	if lastCrawledAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastCrawledAt)
	if err != nil {
		return true
	}
	return time.Since(last) >= interval
}
