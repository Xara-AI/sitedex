package crawler

import (
	"strings"
	"testing"
	"time"
)

func TestParseRobots_BasicDisallow(t *testing.T) {
	txt := `
User-agent: *
Disallow: /admin
Disallow: /cart
`
	r := ParseRobots(strings.NewReader(txt), "sitedex/0.1")
	if r.Allowed("/admin/orders") {
		t.Error("expected /admin/orders to be disallowed")
	}
	if r.Allowed("/cart") {
		t.Error("expected /cart to be disallowed")
	}
	if !r.Allowed("/products/shoes") {
		t.Error("expected /products/shoes to be allowed")
	}
}

func TestParseRobots_AllowOverridesLongerMatch(t *testing.T) {
	txt := `
User-agent: *
Disallow: /products
Allow: /products/public
`
	r := ParseRobots(strings.NewReader(txt), "sitedex")
	if r.Allowed("/products/secret") {
		t.Error("expected /products/secret to be disallowed") // sanity: base case still blocked
	}
	if !r.Allowed("/products/public/item") {
		t.Error("expected /products/public/item to be allowed (longer, more specific Allow wins)")
	}
}

func TestParseRobots_SpecificAgentBeatsWildcard(t *testing.T) {
	txt := `
User-agent: *
Disallow: /

User-agent: sitedex
Disallow: /admin
`
	r := ParseRobots(strings.NewReader(txt), "sitedex/0.1 (+https://github.com/Xara-AI/sitedex)")
	if !r.Allowed("/products/shoes") {
		t.Error("expected sitedex-specific group (which only disallows /admin) to apply, not the wildcard block-all")
	}
	if r.Allowed("/admin") {
		t.Error("expected /admin to still be disallowed for sitedex")
	}
}

func TestParseRobots_GroupedUserAgents(t *testing.T) {
	txt := `
User-agent: a
User-agent: b
Disallow: /x
`
	r := ParseRobots(strings.NewReader(txt), "b-crawler")
	if r.Allowed("/x") {
		t.Error("expected /x to be disallowed for agent b via shared group")
	}
}

func TestParseRobots_CrawlDelay(t *testing.T) {
	txt := `
User-agent: *
Crawl-delay: 2.5
`
	r := ParseRobots(strings.NewReader(txt), "sitedex")
	if r.CrawlDelay() != 2500*time.Millisecond {
		t.Errorf("CrawlDelay() = %v, want 2.5s", r.CrawlDelay())
	}
}

func TestParseRobots_Sitemaps(t *testing.T) {
	txt := `
Sitemap: https://example.com/sitemap.xml
User-agent: *
Disallow:
Sitemap: https://example.com/sitemap-products.xml
`
	r := ParseRobots(strings.NewReader(txt), "sitedex")
	want := []string{"https://example.com/sitemap.xml", "https://example.com/sitemap-products.xml"}
	got := r.Sitemaps()
	if len(got) != len(want) {
		t.Fatalf("Sitemaps() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Sitemaps()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if !r.Allowed("/anything") {
		t.Error("empty Disallow should allow everything")
	}
}

func TestParseRobots_WildcardPattern(t *testing.T) {
	txt := `
User-agent: *
Disallow: /*?sort=
`
	r := ParseRobots(strings.NewReader(txt), "sitedex")
	if r.Allowed("/products?sort=price") {
		t.Error("expected /products?sort=price to be disallowed by wildcard pattern")
	}
	if !r.Allowed("/products?filter=blue") {
		t.Error("expected /products?filter=blue to be allowed")
	}
}

func TestParseRobots_EndAnchor(t *testing.T) {
	txt := `
User-agent: *
Disallow: /file.php$
`
	r := ParseRobots(strings.NewReader(txt), "sitedex")
	if r.Allowed("/file.php") == true {
		t.Error("expected /file.php to be disallowed")
	}
	if !r.Allowed("/file.php?x=1") {
		t.Error("expected /file.php?x=1 to be allowed ($ anchors end of path)")
	}
}

func TestParseRobots_MalformedIsPermissive(t *testing.T) {
	r := ParseRobots(strings.NewReader("not a robots file at all\n\x00\x01garbage"), "sitedex")
	if !r.Allowed("/anything") {
		t.Error("malformed robots.txt should allow everything")
	}
}

func TestParseRobots_NilIsPermissive(t *testing.T) {
	var r *Robots
	if !r.Allowed("/anything") {
		t.Error("nil *Robots should allow everything")
	}
	if r.CrawlDelay() != 0 {
		t.Error("nil *Robots should have zero crawl delay")
	}
}
