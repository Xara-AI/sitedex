package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/extract/content"
)

// recordingWriter is a PageWriter that records pages in memory instead of
// touching disk, keyed by page URL.
type recordingWriter struct {
	mu    sync.Mutex
	pages map[string]*content.Page
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{pages: make(map[string]*content.Page)}
}

func (w *recordingWriter) WritePage(kbDir string, pageURL *url.URL, page *content.Page) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pages[pageURL.String()] = page
	return pageURL.Path, nil
}

func (w *recordingWriter) has(u string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.pages[u]
	return ok
}

func (w *recordingWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pages)
}

func testCrawlConfig(userAgent string) config.CrawlConfig {
	return config.CrawlConfig{
		RateLimitRPS:  1000, // fast for tests
		MaxPages:      100,
		MaxDepth:      5,
		RespectRobots: true,
		UserAgent:     userAgent,
	}
}

func TestCrawler_BasicBFSAndExtraction(t *testing.T) {
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><body><main><h1>Home</h1><p>Welcome to the home page, with enough text to clear the density threshold nicely here.</p><a href="%s/about">About</a></main></body></html>`, baseURL)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><body><main><h1>About</h1><p>About us, with enough content text to clear the density threshold nicely as well.</p></main></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	writer := newRecordingWriter()
	c := New(testCrawlConfig("sitedex-test"), t.TempDir(), writer, nil)

	res, err := c.Crawl(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if res.PagesFetched != 2 {
		t.Errorf("PagesFetched = %d, want 2", res.PagesFetched)
	}
	if !writer.has(srv.URL+"/") || !writer.has(srv.URL+"/about") {
		t.Errorf("expected both / and /about to be written, got: %v", writer.pages)
	}
}

func TestCrawler_RespectsRobotsDisallow(t *testing.T) {
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /secret\n")
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><body><main><p>Home page with enough content text to clear the density threshold nicely here today.</p><a href="%s/secret">Secret</a></main></body></html>`, baseURL)
	})
	mux.HandleFunc("/secret", func(w http.ResponseWriter, r *http.Request) {
		t.Error("robots-disallowed /secret should never be fetched")
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	writer := newRecordingWriter()
	c := New(testCrawlConfig("sitedex-test"), t.TempDir(), writer, nil)

	if _, err := c.Crawl(context.Background(), srv.URL+"/"); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if writer.has(srv.URL + "/secret") {
		t.Error("/secret should not have been written")
	}
}

func TestCrawler_UsesSitemapURLs(t *testing.T) {
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<urlset><url><loc>%s/orphan</loc></url></urlset>`, baseURL)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Home page with enough content text to clear the density threshold nicely here today.</p></main></body></html>`)
	})
	// /orphan is not linked from anywhere, only discoverable via sitemap.xml.
	mux.HandleFunc("/orphan", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Orphan page with enough content text to clear the density threshold nicely as well.</p></main></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	writer := newRecordingWriter()
	c := New(testCrawlConfig("sitedex-test"), t.TempDir(), writer, nil)

	if _, err := c.Crawl(context.Background(), srv.URL+"/"); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if !writer.has(srv.URL + "/orphan") {
		t.Errorf("expected sitemap-only page /orphan to be crawled, got: %v", writer.pages)
	}
}

func TestCrawler_RedirectLoopDoesNotHangCrawl(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop-a", http.StatusFound)
	})
	mux.HandleFunc("/loop-a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop-b", http.StatusFound)
	})
	mux.HandleFunc("/loop-b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop-a", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	writer := newRecordingWriter()
	c := New(testCrawlConfig("sitedex-test"), t.TempDir(), writer, nil)

	done := make(chan struct{})
	var res *Result
	var err error
	go func() {
		res, err = c.Crawl(context.Background(), srv.URL+"/")
		close(done)
	}()
	select {
	case <-done:
		if err != nil {
			t.Fatalf("Crawl: %v", err)
		}
		if res.PagesFailed == 0 {
			t.Error("expected the redirect loop to count as a failed fetch, not hang or silently succeed")
		}
	case <-testTimeout(t):
		t.Fatal("Crawl did not return, likely stuck on the redirect loop")
	}
}

func TestCrawler_ETagRevalidationSkipsUnchangedOnSecondCrawl(t *testing.T) {
	var fetchCount int
	var mu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetchCount++
		mu.Unlock()
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = fmt.Fprint(w, `<html><body><main><p>Stable content that should not change between crawls, long enough to pass density check.</p></main></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dataDir := t.TempDir()
	writer := newRecordingWriter()

	c1 := New(testCrawlConfig("sitedex-test"), dataDir, writer, nil)
	res1, err := c1.Crawl(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("first Crawl: %v", err)
	}
	if res1.PagesFetched != 1 {
		t.Fatalf("first crawl PagesFetched = %d, want 1", res1.PagesFetched)
	}

	c2 := New(testCrawlConfig("sitedex-test"), dataDir, writer, nil)
	res2, err := c2.Crawl(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("second Crawl: %v", err)
	}
	if res2.PagesSkipped != 1 {
		t.Errorf("second crawl PagesSkipped = %d, want 1 (304 Not Modified)", res2.PagesSkipped)
	}
	if res2.PagesFetched != 0 {
		t.Errorf("second crawl PagesFetched = %d, want 0", res2.PagesFetched)
	}

	mu.Lock()
	defer mu.Unlock()
	if fetchCount != 2 {
		t.Errorf("server saw %d requests across two crawls, want 2 (one per crawl)", fetchCount)
	}
}

func TestCrawler_MaxPagesLimit(t *testing.T) {
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	for i := 0; i < 10; i++ {
		i := i
		mux.HandleFunc(fmt.Sprintf("/p%d", i), func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `<html><body><main><p>Page %d with enough content text to clear the density threshold nicely here.</p><a href="%s/p%d">Next</a></main></body></html>`, i, baseURL, (i+1)%10)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	cfg := testCrawlConfig("sitedex-test")
	cfg.MaxPages = 3
	writer := newRecordingWriter()
	c := New(cfg, t.TempDir(), writer, nil)

	res, err := c.Crawl(context.Background(), srv.URL+"/p0")
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if res.PagesVisited > 3 {
		t.Errorf("PagesVisited = %d, want <= 3 (max_pages)", res.PagesVisited)
	}
	if writer.count() > 3 {
		t.Errorf("wrote %d pages, want <= 3", writer.count())
	}
}

func TestCrawler_RateLimitIsHonored(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	var baseURL string
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><body><main><p>Home with enough content text to clear the density threshold nicely.</p><a href="%s/a">a</a><a href="%s/b">b</a></main></body></html>`, baseURL, baseURL)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Page A with enough content text to clear the density threshold nicely.</p></main></body></html>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Page B with enough content text to clear the density threshold nicely.</p></main></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	cfg := testCrawlConfig("sitedex-test")
	cfg.RateLimitRPS = 20 // 50ms between requests to this host
	writer := newRecordingWriter()
	c := New(cfg, t.TempDir(), writer, nil)

	start := time.Now()
	res, err := c.Crawl(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	elapsed := time.Since(start)
	// 3 pages at 50ms/request minimum spacing -> at least ~100ms total.
	if elapsed < 90*time.Millisecond {
		t.Errorf("elapsed = %v, want >= ~100ms given rate_limit_rps=20 across %d pages", elapsed, res.PagesFetched)
	}
}
