package search

import (
	"net"
	"net/url"
	"testing"
)

// hostOf returns urlStr's host with any port stripped, matching how
// crawler.RegistrableDomain keys a site directory for a bare IP:port (see
// internal/crawler/domain.go) — used so tests can seed an index under the
// same site key SearchFresh will look for.
func hostOf(t *testing.T, urlStr string) string {
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
