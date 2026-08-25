package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/index"
)

// A truly unreachable site (RFC 2606's reserved, guaranteed-not-to-resolve
// .example TLD) still degrades gracefully: SearchFresh falls through to
// the cold path (coldpath.go), which can't even fetch a homepage, so it
// returns empty results rather than propagating a fetch error. See
// coldpath_integration_test.go for cold path's actual site-search
// behavior against a reachable site.
func TestSearchFresh_UncrawledSiteReturnsEmptyNotError(t *testing.T) {
	resp, err := New(t.TempDir(), "sitedex-test").SearchFresh(context.Background(), Request{
		Site: "never-crawled.example", Query: "anything", Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if len(resp.Results) != 0 || resp.Source != "index" {
		t.Errorf("resp = %+v, want empty results with source=index", resp)
	}
}

func TestSearchFresh_WithoutFreshIsWarmPathOnly(t *testing.T) {
	dataDir := t.TempDir()
	seedIndexedProduct(t, dataDir, "https://shop.example.com/shoes", "Blue Shoes", 99.0, "in_stock")

	resp, err := New(dataDir, "sitedex-test").SearchFresh(context.Background(), Request{
		Site: "shop.example.com", Query: "blue shoes", Limit: 10, Fresh: false,
	})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "index" {
		t.Errorf("Source = %q, want index (fresh not requested)", resp.Source)
	}
	if len(resp.Results) != 1 || resp.Results[0].Verified {
		t.Errorf("results = %+v, want 1 unverified result", resp.Results)
	}
}

func TestSearchFresh_VerifiesAndUpdatesPrice(t *testing.T) {
	var liveURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><head><script type="application/ld+json">
{"@type":"Product","name":"Blue Shoes","offers":{"@type":"Offer","price":"149.00","priceCurrency":"USD","availability":"https://schema.org/OutOfStock"}}
</script></head><body></body></html>`)
	}))
	defer srv.Close()
	liveURL = srv.URL

	dataDir := t.TempDir()
	seedIndexedProduct(t, dataDir, liveURL, "Blue Shoes", 99.0, "in_stock")

	resp, err := New(dataDir, "sitedex-test").SearchFresh(context.Background(), Request{
		Site: "127.0.0.1", Query: "blue shoes", Limit: 10, Fresh: true, FreshTopN: 3, FreshTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "index+live" {
		t.Fatalf("Source = %q, want index+live", resp.Source)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results = %+v, want 1", resp.Results)
	}
	r := resp.Results[0]
	if !r.Verified {
		t.Error("expected Verified=true")
	}
	if r.VerifiedAt.IsZero() {
		t.Error("expected VerifiedAt to be set")
	}
	if r.Price != 149.0 || r.Availability != "out_of_stock" {
		t.Errorf("Price=%v Availability=%q, want the freshly-fetched 149.0/out_of_stock (not the stale indexed 99.0/in_stock)", r.Price, r.Availability)
	}

	// The index itself should also have been updated (not just the response).
	idx, err := index.Open(dataDir, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idx.Close() }()
	stats, err := idx.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.ProductCount != 1 {
		t.Errorf("ProductCount = %d, want 1", stats.ProductCount)
	}
}

func TestSearchFresh_RespectsTimeoutBudget(t *testing.T) {
	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh // never unblocks during the test; simulates a slow/hanging upstream
	}))
	defer srv.Close()
	defer close(blockCh) // let the handler goroutine exit after the test finishes

	dataDir := t.TempDir()
	seedIndexedProduct(t, dataDir, srv.URL, "Blue Shoes", 99.0, "in_stock")

	start := time.Now()
	done := make(chan Response, 1)
	var searchErr error
	go func() {
		resp, err := New(dataDir, "sitedex-test").SearchFresh(context.Background(), Request{
			Site: "127.0.0.1", Query: "blue shoes", Limit: 10, Fresh: true, FreshTopN: 1, FreshTimeout: 200 * time.Millisecond,
		})
		searchErr = err
		done <- resp
	}()

	select {
	case resp := <-done:
		elapsed := time.Since(start)
		if searchErr != nil {
			t.Fatalf("SearchFresh: %v", searchErr)
		}
		if elapsed > 1*time.Second {
			t.Errorf("elapsed = %v, want well under 1s given a 200ms fresh_timeout against a hanging upstream", elapsed)
		}
		if resp.Source != "index" {
			t.Errorf("Source = %q, want index (verification never completed in time)", resp.Source)
		}
		if len(resp.Results) != 1 || resp.Results[0].Verified {
			t.Errorf("results = %+v, want the original unverified indexed result, not blocked or stale-verified", resp.Results)
		}
		if resp.Results[0].Price != 99.0 {
			t.Errorf("Price = %v, want the original indexed 99.0 (fresh fetch never returned in time)", resp.Results[0].Price)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SearchFresh did not return within 5s, despite a 200ms fresh_timeout — response budget was not honored")
	}
}

// TestSearchFresh_SoftFindsInflectedMatchStrictMisses reproduces the
// production incident end to end through SearchFresh: an exact query with
// Romanian grammatical agreement the indexed copy doesn't share ("externalizată"
// vs. indexed "externalizat") returns nothing without Soft, and the
// suffix-relaxed match with it — same contract as index.DB.SearchSoft, just
// exercised through the request path the HTTP API actually uses.
func TestSearchFresh_SoftFindsInflectedMatchStrictMisses(t *testing.T) {
	dataDir := t.TempDir()
	site := hostOf(t, "https://agency.example.com/echipa-marketing")
	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	if err := idx.IndexPage(index.PageRecord{
		URL: "https://agency.example.com/echipa-marketing", Title: "Echipa tehnică de marketing externalizat", CrawledAt: time.Now(),
	}, []index.ChunkRecord{
		{Ordinal: 0, HeadingPath: "Echipa tehnică de marketing externalizat", Text: "Serviciul nostru de echipă de marketing externalizat acoperă strategie, conținut și performanță."},
	}); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	_ = idx.Close()

	strict, err := New(dataDir, "sitedex-test").SearchFresh(context.Background(), Request{
		Site: site, Query: "echipă marketing externalizată", Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchFresh (strict): %v", err)
	}
	if len(strict.Results) != 0 {
		t.Fatalf("strict SearchFresh = %+v, want 0 results", strict.Results)
	}

	soft, err := New(dataDir, "sitedex-test").SearchFresh(context.Background(), Request{
		Site: site, Query: "echipă marketing externalizată", Limit: 10, Soft: true,
	})
	if err != nil {
		t.Fatalf("SearchFresh (soft): %v", err)
	}
	if len(soft.Results) != 1 || soft.Results[0].URL != "https://agency.example.com/echipa-marketing" {
		t.Fatalf("soft SearchFresh = %+v, want the echipa-marketing page", soft.Results)
	}
}

// seedIndexedProduct opens (creating) an index for the registrable domain
// of urlStr and indexes a single product page.
func seedIndexedProduct(t *testing.T, dataDir, urlStr, name string, price float64, availability string) {
	t.Helper()
	site := hostOf(t, urlStr)
	idx, err := index.Open(dataDir, site)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = idx.Close() }()

	if err := idx.IndexPage(index.PageRecord{URL: urlStr, Title: name, CrawledAt: time.Now()}, nil); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	err = idx.IndexProduct(urlStr, &index.ProductRecord{
		Name: name, Price: price, HasPrice: true, Currency: "USD", Availability: availability, ExtractionMethod: "json-ld",
	})
	if err != nil {
		t.Fatalf("IndexProduct: %v", err)
	}
}
