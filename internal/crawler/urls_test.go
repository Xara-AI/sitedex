package crawler

import (
	"net/url"
	"testing"
)

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://Example.com/Path/", "https://example.com/Path"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com", "https://example.com/"},
		{"https://example.com:443/x", "https://example.com/x"},
		{"http://example.com:80/x", "http://example.com/x"},
		{"https://example.com/x#section", "https://example.com/x"},
		{"https://example.com:8443/x", "https://example.com:8443/x"},
	}
	for _, tc := range cases {
		got := NormalizeURL(mustParseURL(t, tc.in)).String()
		if got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	base := mustParseURL(t, "https://example.com/blog/post-1")
	raw := []byte(`
<html><body>
<nav><a href="/">Home</a><a href="/products">Products</a></nav>
<article>
  <a href="https://example.com/blog/post-2">Next post</a>
  <a href="../about">About</a>
  <a href="#top">Back to top</a>
  <a href="mailto:hi@example.com">Email</a>
  <a href="javascript:void(0)">JS</a>
  <a href="https://other.com/x">External</a>
</article>
</body></html>`)

	links := ExtractLinks(base, raw)
	want := map[string]bool{
		"https://example.com/":            true,
		"https://example.com/products":    true,
		"https://example.com/blog/post-2": true,
		"https://example.com/about":       true,
		"https://other.com/x":             true,
	}
	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d: %v", len(links), len(want), links)
	}
	for _, l := range links {
		if !want[l.String()] {
			t.Errorf("unexpected link %q", l.String())
		}
	}
}

func TestExtractLinks_MalformedHTMLDoesNotPanic(t *testing.T) {
	base := mustParseURL(t, "https://example.com/")
	_ = ExtractLinks(base, []byte("<html><body><a href=/broken<<<>"))
}
