package search

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseDoc(t *testing.T, raw string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	return doc
}

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestBuildSearchURL_WooCommerce(t *testing.T) {
	doc := parseDoc(t, `<html><body class="woocommerce"><div class="product">x</div></body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "blue shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/?s=blue+shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_Shopify(t *testing.T) {
	doc := parseDoc(t, `<html><head><script src="https://cdn.shopify.com/s/files/1/theme.js"></script></head><body></body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "blue shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/search?q=blue+shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_PrestaShop(t *testing.T) {
	doc := parseDoc(t, `<html><head><meta name="generator" content="PrestaShop"></head><body></body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "blue shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/search?controller=search&s=blue+shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_OpenCart(t *testing.T) {
	doc := parseDoc(t, `<html><head><meta name="generator" content="OpenCart"></head><body></body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "blue shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/index.php?route=product%2Fsearch&search=blue+shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_GenericFormFallback(t *testing.T) {
	doc := parseDoc(t, `<html><body>
<form role="search" action="/find" method="GET">
  <input type="search" name="query">
</form>
</body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "blue shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/find?query=blue+shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_GenericFormClassFallback(t *testing.T) {
	doc := parseDoc(t, `<html><body>
<form class="search-form" action="/search-here">
  <input type="text" name="q">
</form>
</body></html>`)
	got, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "shoes")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "https://shop.example.com/search-here?q=shoes" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSearchURL_PostFormIsRejected(t *testing.T) {
	doc := parseDoc(t, `<html><body>
<form role="search" action="/find" method="POST">
  <input type="search" name="query">
</form>
</body></html>`)
	_, ok := buildSearchURL(doc, mustParseURL(t, "https://shop.example.com/"), "shoes")
	if ok {
		t.Error("expected ok=false for a POST-only search form")
	}
}

func TestBuildSearchURL_NothingFound(t *testing.T) {
	doc := parseDoc(t, `<html><body><p>Just a plain page, no commerce markers or search form.</p></body></html>`)
	_, ok := buildSearchURL(doc, mustParseURL(t, "https://example.com/"), "shoes")
	if ok {
		t.Error("expected ok=false when no platform or search form is detected")
	}
}
