package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/index"
)

func TestIsStale(t *testing.T) {
	cases := []struct {
		name          string
		lastCrawledAt string
		interval      time.Duration
		want          bool
	}{
		{"empty timestamp", "", time.Hour, true},
		{"unparsable timestamp", "not-a-time", time.Hour, true},
		{"well within interval", time.Now().Add(-1 * time.Minute).Format(time.RFC3339), time.Hour, false},
		{"past interval", time.Now().Add(-2 * time.Hour).Format(time.RFC3339), time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStale(tc.lastCrawledAt, tc.interval); got != tc.want {
				t.Errorf("isStale(%q, %v) = %v, want %v", tc.lastCrawledAt, tc.interval, got, tc.want)
			}
		})
	}
}

func TestRecrawlStaleSites_TriggersJobForStaleSite(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Home page content long enough to clear the density threshold for this scheduler test.</p></main></body></html>`)
	})
	target := httptest.NewServer(mux)
	defer target.Close()
	site := hostOnly(t, target.URL)

	dataDir := t.TempDir()
	// Seed an index for this site with a stale crawled_at.
	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatal(err)
	}
	err = idx.IndexPage(index.PageRecord{
		URL: target.URL + "/", Title: "Old", CrawledAt: time.Now().Add(-48 * time.Hour),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(dataDir)
	cfg.Crawl.RecrawlInterval = config.Duration(time.Nanosecond) // so "48h old" is always stale
	srv := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	srv.recrawlStaleSites(context.Background())

	// recrawlStaleSites enqueues the job asynchronously (go s.runCrawlJob);
	// poll briefly for it to land and complete.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		srv.jobs.mu.Lock()
		for _, j := range srv.jobs.jobs {
			if j.state == jobSucceeded {
				found = true
			}
		}
		n := len(srv.jobs.jobs)
		srv.jobs.mu.Unlock()
		if found {
			break
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}

	srv.jobs.mu.Lock()
	defer srv.jobs.mu.Unlock()
	if len(srv.jobs.jobs) != 1 {
		t.Fatalf("jobs = %v, want exactly 1 triggered", srv.jobs.jobs)
	}
	for _, j := range srv.jobs.jobs {
		if j.state != jobSucceeded {
			t.Errorf("job state = %q, want succeeded (err: %s)", j.state, j.err)
		}
	}
}

func TestRecrawlStaleSites_SkipsFreshSite(t *testing.T) {
	dataDir := t.TempDir()
	idx, err := index.Open(dataDir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexPage(index.PageRecord{URL: "https://example.com/", Title: "Fresh", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(dataDir)
	cfg.Crawl.RecrawlInterval = config.Duration(1000 * time.Second) // much longer than "just now"
	srv := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	srv.recrawlStaleSites(context.Background())

	time.Sleep(20 * time.Millisecond) // let any (unwanted) goroutine have a chance to register
	srv.jobs.mu.Lock()
	defer srv.jobs.mu.Unlock()
	if len(srv.jobs.jobs) != 0 {
		t.Errorf("jobs = %v, want none triggered for a freshly-crawled site", srv.jobs.jobs)
	}
}
