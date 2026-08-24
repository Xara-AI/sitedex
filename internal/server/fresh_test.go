package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestServer_SearchFreshVerifiesAndReturnsVerifiedAt(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><head><script type="application/ld+json">
{"@type":"Product","name":"Blue Shoes","offers":{"@type":"Offer","price":"79.00","priceCurrency":"USD","availability":"https://schema.org/OutOfStock"}}
</script></head><body></body></html>`)
	}))
	defer target.Close()

	dataDir := t.TempDir()
	site := hostOnly(t, target.URL)
	seedTestProduct(t, dataDir, site, target.URL+"/", "Blue Shoes", 99.99, "in_stock")

	cfg := testConfig(dataDir)
	base, stop := testServer(t, cfg)
	defer stop()

	body := fmt.Sprintf(`{"site":%q,"query":"blue shoes","fresh":true}`, site)
	resp, err := http.Post(base+"/v1/search", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Source != "index+live" {
		t.Fatalf("Source = %q, want index+live", out.Source)
	}
	if len(out.Results) != 1 {
		t.Fatalf("Results = %+v, want 1", out.Results)
	}
	r := out.Results[0]
	if r.VerifiedAt == nil {
		t.Error("expected verified_at to be present")
	}
	if r.Price == nil || *r.Price != 79.0 {
		t.Errorf("Price = %v, want the freshly-fetched 79.0", r.Price)
	}
	if r.Availability != "out_of_stock" {
		t.Errorf("Availability = %q, want out_of_stock", r.Availability)
	}
}

// hostOnly returns urlStr's host with any port stripped, matching how
// crawler.RegistrableDomain keys a site directory for a bare IP:port.
func hostOnly(t *testing.T, urlStr string) string {
	t.Helper()
	u, err := url.Parse(urlStr)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", urlStr, err)
	}
	if host, _, err := net.SplitHostPort(u.Host); err == nil {
		return host
	}
	return u.Host
}
