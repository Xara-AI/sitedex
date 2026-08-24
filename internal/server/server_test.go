package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/index"
)

// testServer starts a Server on a loopback listener (OS-assigned free
// port) in the background and returns its base URL and a stop function
// that cancels its context and waits for serve() to return — no real
// signals, no fixed ports, no sleeping-and-hoping.
func testServer(t *testing.T, cfg *config.Config) (baseURL string, stop func()) {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, logger)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.serve(ctx, ln) }()

	stop = func() {
		cancel()
		// serve()'s own internal shutdown grace period is 15s (see
		// server.go); this must wait comfortably longer than that or a
		// legitimate (if slow) graceful shutdown fails the test instead
		// of a genuine hang. 20s under -race/loaded CI runners, not 5s.
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve() returned error after shutdown: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("serve() did not return within 20s of context cancellation")
		}
	}
	return "http://" + ln.Addr().String(), stop
}

func testConfig(dataDir string) *config.Config {
	cfg := config.Default()
	cfg.DataDir = dataDir
	cfg.Crawl.RateLimitRPS = 1000
	cfg.Crawl.RecrawlInterval = 0 // disable the scheduler in tests unless a test opts in
	return cfg
}

func TestServer_Healthz(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	_, stop := testServer(t, testConfig(t.TempDir()))
	stop() // the test itself is the assertion: stop() fails the test if serve() doesn't return in time
}

func TestServer_SearchEmptyResultsIsNotAnError(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	body := `{"site":"never-crawled.example","query":"anything"}`
	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// nil vs empty slice marshal to `null` vs `[]` respectively; what
	// matters here is there's no error and zero results, not which one.
	if len(out.Results) != 0 {
		t.Errorf("Results = %+v, want empty", out.Results)
	}
	if out.Source != "index" {
		t.Errorf("Source = %q, want index", out.Source)
	}
}

func TestServer_SearchValidation(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	cases := []struct {
		name string
		body string
	}{
		{"missing site", `{"query":"x"}`},
		{"missing query", `{"site":"example.com"}`},
		{"invalid json", `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestServer_SearchFindsIndexedProduct(t *testing.T) {
	dataDir := t.TempDir()
	seedTestProduct(t, dataDir, "example.com", "https://example.com/shoes", "Blue Shoes", 99.99, "in_stock")

	base, stop := testServer(t, testConfig(dataDir))
	defer stop()

	body := `{"site":"example.com","query":"blue shoes"}`
	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("Results = %+v, want 1", out.Results)
	}
	r := out.Results[0]
	if r.Type != "product" || r.Title != "Blue Shoes" {
		t.Errorf("got Type=%q Title=%q", r.Type, r.Title)
	}
	if r.Price == nil || *r.Price != 99.99 {
		t.Errorf("Price = %v, want 99.99", r.Price)
	}
	if r.VerifiedAt != nil {
		t.Error("VerifiedAt should be absent when fresh was not requested")
	}
}

func TestServer_SearchTypeFilter(t *testing.T) {
	dataDir := t.TempDir()
	idx, err := index.Open(dataDir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexPage(index.PageRecord{URL: "https://example.com/guide", Title: "Shoe Guide", CrawledAt: time.Now()},
		[]index.ChunkRecord{{Ordinal: 0, Text: "A guide about running shoes."}}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexProduct("https://example.com/guide", nil); err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	seedTestProduct(t, dataDir, "example.com", "https://example.com/shoes", "Running Shoes", 50, "in_stock")

	base, stop := testServer(t, testConfig(dataDir))
	defer stop()

	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(`{"site":"example.com","query":"running shoes","type":"page"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].Type != "page" {
		t.Errorf("results = %+v, want 1 page-type result", out.Results)
	}
}

func TestServer_CrawlTriggerAndStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><main><p>Home page content long enough to clear the density threshold nicely for this server test.</p></main></body></html>`)
	})
	target := httptest.NewServer(mux)
	defer target.Close()

	cfg := testConfig(t.TempDir())
	base, stop := testServer(t, cfg)
	defer stop()

	triggerBody := fmt.Sprintf(`{"site":%q}`, target.URL+"/")
	resp, err := http.Post(base+"/v1/crawl", "application/json", bytes.NewBufferString(triggerBody))
	if err != nil {
		t.Fatalf("POST /v1/crawl: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var triggered struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&triggered); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if triggered.JobID == "" {
		t.Fatal("expected a non-empty job_id")
	}

	// Poll GET /v1/crawl/{job} until it's no longer queued/running.
	var status jobStatusDTO
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(base + "/v1/crawl/" + triggered.JobID)
		if err != nil {
			t.Fatalf("GET /v1/crawl/%s: %v", triggered.JobID, err)
		}
		if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = r.Body.Close()
		if status.Status == jobSucceeded || status.Status == jobFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Status != jobSucceeded {
		t.Fatalf("job status = %+v, want succeeded", status)
	}
	if status.Result == nil || status.Result.PagesFetched != 1 {
		t.Errorf("Result = %+v, want PagesFetched=1", status.Result)
	}
}

func TestServer_CrawlStatusUnknownJob(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	resp, err := http.Get(base + "/v1/crawl/job-does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_Sites(t *testing.T) {
	dataDir := t.TempDir()
	seedTestProduct(t, dataDir, "example.com", "https://example.com/shoes", "Blue Shoes", 99, "in_stock")

	base, stop := testServer(t, testConfig(dataDir))
	defer stop()

	resp, err := http.Get(base + "/v1/sites")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Sites []siteDTO `json:"sites"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sites) != 1 || out.Sites[0].Site != "example.com" || out.Sites[0].Products != 1 {
		t.Errorf("sites = %+v", out.Sites)
	}
}

func TestServer_Metrics(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	// Generate at least one search so the counters are non-zero.
	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(`{"site":"nonexistent.invalid","query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	mresp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mresp.Body.Close() }()
	data, err := io.ReadAll(mresp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"sitedex_up 1", "sitedex_search_requests_total 1", "sitedex_search_duration_seconds_bucket"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q; got:\n%s", want, body)
		}
	}
}

func TestServer_AuthRequiredWhenTokenSet(t *testing.T) {
	cfg := testConfig(t.TempDir())
	cfg.Token = "secret123"
	base, stop := testServer(t, cfg)
	defer stop()

	// No Authorization header: rejected.
	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(`{"site":"nonexistent.invalid","query":"y"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status without token = %d, want 401", resp.StatusCode)
	}

	// healthz is exempt from auth.
	hresp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = hresp.Body.Close()
	if hresp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want 200 even without a token", hresp.StatusCode)
	}

	// Correct bearer token: allowed.
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/search", bytes.NewBufferString(`{"site":"nonexistent.invalid","query":"y"}`))
	req.Header.Set("Authorization", "Bearer secret123")
	req.Header.Set("Content-Type", "application/json")
	aresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = aresp.Body.Close()
	if aresp.StatusCode != http.StatusOK {
		t.Errorf("status with correct token = %d, want 200", aresp.StatusCode)
	}
}

func seedTestProduct(t *testing.T, dataDir, site, url, name string, price float64, availability string) {
	t.Helper()
	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = idx.Close() }()
	if err := idx.IndexPage(index.PageRecord{URL: url, Title: name, CrawledAt: time.Now()}, nil); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	err = idx.IndexProduct(url, &index.ProductRecord{
		Name: name, Price: price, HasPrice: true, Currency: "USD", Availability: availability, ExtractionMethod: "json-ld",
	})
	if err != nil {
		t.Fatalf("IndexProduct: %v", err)
	}
}
