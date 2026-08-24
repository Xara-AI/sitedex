package server

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Xara-AI/sitedex/internal/index"
)

// latencyBucketBounds are the histogram bucket upper bounds, in seconds,
// for sitedex_search_duration_seconds. Chosen to resolve the sub-second
// range this tool actually operates in (a chat-latency-compatible search
// should land well under 1s even with fresh-verify).
var latencyBucketBounds = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

// metrics holds the counters/histogram exposed at GET /metrics, in
// Prometheus text exposition format — hand-rolled rather than pulling in
// a client library, per CLAUDE.md's minimal-dependencies constraint (the
// format itself is simple plain text).
type metrics struct {
	searchRequests  atomic.Int64
	searchErrors    atomic.Int64
	crawlsSucceeded atomic.Int64
	crawlsFailed    atomic.Int64

	mu      sync.Mutex
	buckets map[float64]int64
	sum     float64
	count   int64
}

func newMetrics() *metrics {
	m := &metrics{buckets: make(map[float64]int64, len(latencyBucketBounds))}
	for _, b := range latencyBucketBounds {
		m.buckets[b] = 0
	}
	return m
}

func (m *metrics) observeSearch(d time.Duration, failed bool) {
	m.searchRequests.Add(1)
	if failed {
		m.searchErrors.Add(1)
	}

	seconds := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sum += seconds
	m.count++
	for _, b := range latencyBucketBounds {
		if seconds <= b {
			m.buckets[b]++
		}
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.metrics.writeTo(w)

	_, _ = fmt.Fprint(w, "# HELP sitedex_index_pages Number of indexed pages, by site.\n# TYPE sitedex_index_pages gauge\n")
	_, _ = fmt.Fprint(w, "# HELP sitedex_index_chunks Number of indexed chunks, by site.\n# TYPE sitedex_index_chunks gauge\n")
	_, _ = fmt.Fprint(w, "# HELP sitedex_index_products Number of indexed products, by site.\n# TYPE sitedex_index_products gauge\n")
	sites, err := index.ListSites(s.cfg.DataDir)
	if err == nil {
		for _, site := range sites {
			stats, err := s.siteStats(site)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "sitedex_index_pages{site=%q} %d\n", site, stats.PageCount)
			_, _ = fmt.Fprintf(w, "sitedex_index_chunks{site=%q} %d\n", site, stats.ChunkCount)
			_, _ = fmt.Fprintf(w, "sitedex_index_products{site=%q} %d\n", site, stats.ProductCount)
		}
	}
}

func (m *metrics) writeTo(w io.Writer) {
	_, _ = fmt.Fprint(w, "# HELP sitedex_up Whether the server process is up.\n# TYPE sitedex_up gauge\nsitedex_up 1\n")

	_, _ = fmt.Fprint(w, "# HELP sitedex_search_requests_total Total search requests handled.\n# TYPE sitedex_search_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "sitedex_search_requests_total %d\n", m.searchRequests.Load())

	_, _ = fmt.Fprint(w, "# HELP sitedex_search_errors_total Total search requests that errored.\n# TYPE sitedex_search_errors_total counter\n")
	_, _ = fmt.Fprintf(w, "sitedex_search_errors_total %d\n", m.searchErrors.Load())

	_, _ = fmt.Fprint(w, "# HELP sitedex_crawls_total Total crawls, by outcome.\n# TYPE sitedex_crawls_total counter\n")
	_, _ = fmt.Fprintf(w, "sitedex_crawls_total{status=\"succeeded\"} %d\n", m.crawlsSucceeded.Load())
	_, _ = fmt.Fprintf(w, "sitedex_crawls_total{status=\"failed\"} %d\n", m.crawlsFailed.Load())

	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = fmt.Fprint(w, "# HELP sitedex_search_duration_seconds Search request latency.\n# TYPE sitedex_search_duration_seconds histogram\n")
	for _, b := range latencyBucketBounds {
		_, _ = fmt.Fprintf(w, "sitedex_search_duration_seconds_bucket{le=%q} %d\n", strconv.FormatFloat(b, 'g', -1, 64), m.buckets[b])
	}
	_, _ = fmt.Fprintf(w, "sitedex_search_duration_seconds_bucket{le=\"+Inf\"} %d\n", m.count)
	_, _ = fmt.Fprintf(w, "sitedex_search_duration_seconds_sum %s\n", strconv.FormatFloat(m.sum, 'g', -1, 64))
	_, _ = fmt.Fprintf(w, "sitedex_search_duration_seconds_count %d\n", m.count)
}
