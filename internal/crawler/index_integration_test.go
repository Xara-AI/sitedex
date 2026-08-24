package crawler_test

// This file lives in the crawler_test (external) package specifically so
// it can import internal/index without creating an internal/crawler <->
// internal/index dependency in non-test code — the production Indexer
// interface (see crawler.go) is what keeps that dependency one-way.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/extract/content"
	"github.com/Xara-AI/sitedex/internal/index"
)

type indexAdapter struct{ db *index.DB }

func (a indexAdapter) IndexPage(page crawler.PageForIndex, chunks []crawler.ChunkForIndex) error {
	rec := index.PageRecord{
		URL: page.URL, Title: page.Title, Description: page.Description, Lang: page.Lang,
		Hash: page.Hash, CrawledAt: page.CrawledAt, ETag: page.ETag, LastModified: page.LastModified,
	}
	chunkRecs := make([]index.ChunkRecord, len(chunks))
	for i, c := range chunks {
		chunkRecs[i] = index.ChunkRecord{Ordinal: c.Ordinal, HeadingPath: c.HeadingPath, Text: c.Text}
	}
	return a.db.IndexPage(rec, chunkRecs)
}

func (a indexAdapter) IndexProduct(pageURL string, p *crawler.ProductForIndex) error {
	if p == nil {
		return a.db.IndexProduct(pageURL, nil)
	}
	return a.db.IndexProduct(pageURL, &index.ProductRecord{
		Name: p.Name, Description: p.Description, Price: p.Price, HasPrice: p.HasPrice,
		Currency: p.Currency, Availability: p.Availability, Image: p.Image,
		ExtractionMethod: p.ExtractionMethod, RawJSON: p.RawJSON,
	})
}

// noopWriter discards markdown output — this test only cares about the
// index side of the pipeline.
type noopWriter struct{}

func (noopWriter) WritePage(kbDir string, pageURL *url.URL, page *content.Page) (string, error) {
	return "", nil
}

func TestCrawlerIndexesPagesIntoSearchableIndex(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><main><h1>Blue Nike Shoes</h1><p>Lightweight running shoes in blue, built for long distances and everyday training.</p></main></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dataDir := t.TempDir()
	site, err := crawler.SiteForURL(srv.URL)
	if err != nil {
		t.Fatalf("SiteForURL: %v", err)
	}

	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = idx.Close() }()

	cfg := config.CrawlConfig{RateLimitRPS: 1000, MaxPages: 5, MaxDepth: 1, RespectRobots: true, UserAgent: "sitedex-test"}
	chunking := config.ChunkingConfig{TargetChars: 1200, OverlapChars: 100}

	c := crawler.New(cfg, chunking, config.LLMExtractorConfig{}, dataDir, noopWriter{}, indexAdapter{idx}, nil)
	if _, err := c.Crawl(context.Background(), srv.URL+"/"); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	results, err := idx.Search("blue shoes", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1 (the crawled page should be searchable)", results)
	}
	if results[0].Title != "Blue Nike Shoes" {
		t.Errorf("results[0].Title = %q, want Blue Nike Shoes", results[0].Title)
	}

	stats, err := idx.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PageCount != 1 || stats.ChunkCount == 0 {
		t.Errorf("stats = %+v, want 1 page and > 0 chunks", stats)
	}
}

func TestCrawlerIndexesProductsIntoSearchableIndex(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><main><p>Welcome to the shop, long enough to clear the content density threshold nicely.</p></main></body></html>`))
	})
	mux.HandleFunc("/product/blue-shoes", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><script type="application/ld+json">
{"@context":"https://schema.org/","@type":"Product","name":"Blue Running Shoes",
 "offers":{"@type":"Offer","price":"129.99","priceCurrency":"USD","availability":"https://schema.org/InStock"}}
</script></head><body><main><h1>Blue Running Shoes</h1><a href="/">Home</a></main></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dataDir := t.TempDir()
	site, err := crawler.SiteForURL(srv.URL)
	if err != nil {
		t.Fatalf("SiteForURL: %v", err)
	}
	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = idx.Close() }()

	cfg := config.CrawlConfig{RateLimitRPS: 1000, MaxPages: 5, MaxDepth: 2, RespectRobots: true, UserAgent: "sitedex-test"}
	chunking := config.ChunkingConfig{TargetChars: 1200, OverlapChars: 100}

	c := crawler.New(cfg, chunking, config.LLMExtractorConfig{}, dataDir, noopWriter{}, indexAdapter{idx}, nil)
	if _, err := c.Crawl(context.Background(), srv.URL+"/product/blue-shoes"); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	results, err := idx.Search("blue running shoes", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	r := results[0]
	if r.Type != "product" {
		t.Errorf("Type = %q, want product", r.Type)
	}
	if !r.HasPrice || r.Price != 129.99 || r.Currency != "USD" {
		t.Errorf("Price=%v HasPrice=%v Currency=%q, want 129.99/true/USD", r.Price, r.HasPrice, r.Currency)
	}
	if r.Availability != "in_stock" {
		t.Errorf("Availability = %q, want in_stock", r.Availability)
	}
	if r.ExtractionMethod != "json-ld" {
		t.Errorf("ExtractionMethod = %q, want json-ld", r.ExtractionMethod)
	}

	stats, err := idx.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.ProductCount != 1 {
		t.Errorf("ProductCount = %d, want 1", stats.ProductCount)
	}
}
