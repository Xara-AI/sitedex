package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// withLocalSiteBase points siteBaseURL at srv's own URL for the duration
// of the test, so coldSearch's hardcoded "assume https" resolution
// targets a local httptest server instead of trying (and failing) to
// reach a real site over TLS.
func withLocalSiteBase(t *testing.T, srv *httptest.Server) {
	t.Helper()
	base, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", srv.URL, err)
	}
	orig := siteBaseURL
	siteBaseURL = func(site string) (*url.URL, error) { return base, nil }
	t.Cleanup(func() { siteBaseURL = orig })
}

func TestSearchFresh_ColdPathWooCommerce(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("s") == "shoes" {
			_, _ = fmt.Fprint(w, `<html><body class="woocommerce"><ul class="products">
<li class="product"><a href="/product/blue-shoes/"><h2 class="woocommerce-loop-product__title">Blue Shoes</h2>
<span class="price"><span class="woocommerce-Price-amount amount"><bdi>99.00</bdi></span></span></a></li>
</ul></body></html>`)
			return
		}
		_, _ = fmt.Fprint(w, `<html><body class="woocommerce"><div class="product">home</div></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withLocalSiteBase(t, srv)

	site := hostOf(t, srv.URL)
	searcher := New(t.TempDir(), "sitedex-test")

	resp, err := searcher.SearchFresh(context.Background(), Request{Site: site, Query: "shoes", Limit: 10})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "site-search" {
		t.Fatalf("Source = %q, want site-search; results=%+v", resp.Source, resp.Results)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("Results = %+v, want 1", resp.Results)
	}
	r := resp.Results[0]
	if r.Title != "Blue Shoes" || r.Type != "product" {
		t.Errorf("got Title=%q Type=%q", r.Title, r.Type)
	}
	if !r.HasPrice || r.Price != 99.0 {
		t.Errorf("Price = %v/%v, want 99.0", r.Price, r.HasPrice)
	}
	if r.URL != srv.URL+"/product/blue-shoes/" {
		t.Errorf("URL = %q", r.URL)
	}
}

func TestSearchFresh_ColdPathRespectsRobotsDisallow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Error("robots-disallowed homepage should never be fetched by cold path")
		_, _ = fmt.Fprint(w, `<html><body class="woocommerce"><div class="product">home</div></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withLocalSiteBase(t, srv)

	site := hostOf(t, srv.URL)
	resp, err := New(t.TempDir(), "sitedex-test").SearchFresh(context.Background(), Request{Site: site, Query: "shoes", Limit: 10})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "index" || len(resp.Results) != 0 {
		t.Errorf("resp = %+v, want empty results with source=index (robots.txt disallows /)", resp)
	}
}

func TestSearchFresh_ColdPathRespectsRobotsDisallowOnSearchPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /search\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			t.Error("robots-disallowed /search should never be fetched by cold path")
		}
		_, _ = fmt.Fprint(w, `<html><head><script src="https://cdn.shopify.com/s/theme.js"></script></head><body></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withLocalSiteBase(t, srv)

	site := hostOf(t, srv.URL)
	resp, err := New(t.TempDir(), "sitedex-test").SearchFresh(context.Background(), Request{Site: site, Query: "shoes", Limit: 10})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "index" || len(resp.Results) != 0 {
		t.Errorf("resp = %+v, want empty results with source=index (robots.txt disallows /search)", resp)
	}
}

func TestSearchFresh_ColdPathNoMechanismFoundDegradesToEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><p>Nothing here — no platform markers, no search form.</p></body></html>`)
	}))
	defer srv.Close()
	withLocalSiteBase(t, srv)

	site := hostOf(t, srv.URL)
	searcher := New(t.TempDir(), "sitedex-test")

	resp, err := searcher.SearchFresh(context.Background(), Request{Site: site, Query: "shoes", Limit: 10})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "index" || len(resp.Results) != 0 {
		t.Errorf("resp = %+v, want empty results with source=index", resp)
	}
}

// (Unreachable-site degradation is covered by
// TestSearchFresh_UncrawledSiteReturnsEmptyNotError in fresh_test.go.)

func TestSearchFresh_ForceSiteSearchBypassesExistingIndex(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("s") == "shoes" {
			_, _ = fmt.Fprint(w, `<html><body class="woocommerce"><ul class="products">
<li class="product"><a href="/product/fresh-shoes/"><h2 class="woocommerce-loop-product__title">Freshly Live Shoes</h2>
<span class="price"><span class="woocommerce-Price-amount amount"><bdi>199.00</bdi></span></span></a></li>
</ul></body></html>`)
			return
		}
		_, _ = fmt.Fprint(w, `<html><body class="woocommerce"><div class="product">home</div></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withLocalSiteBase(t, srv)

	site := hostOf(t, srv.URL)
	dataDir := t.TempDir()
	// Seed a stale index that would normally answer the warm path.
	seedIndexedProduct(t, dataDir, srv.URL+"/", "Stale Indexed Shoes", 1.0, "unknown")

	searcher := New(dataDir, "sitedex-test")
	resp, err := searcher.SearchFresh(context.Background(), Request{
		Site: site, Query: "shoes", Limit: 10, ForceSiteSearch: true,
	})
	if err != nil {
		t.Fatalf("SearchFresh: %v", err)
	}
	if resp.Source != "site-search" {
		t.Fatalf("Source = %q, want site-search even though an index exists", resp.Source)
	}
	if len(resp.Results) != 1 || resp.Results[0].Title != "Freshly Live Shoes" {
		t.Errorf("results = %+v, want the live site-search result, not the stale indexed one", resp.Results)
	}
}
