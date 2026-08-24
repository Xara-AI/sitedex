package export

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/extract/content"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestSlugForURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://example.com/", "index.md"},
		{"https://example.com", "index.md"},
		{"https://example.com/products/blue-shoes", "products/blue-shoes.md"},
		{"https://example.com/products/blue-shoes?x=1&y=2", "products/blue-shoes.md"},
		{"https://example.com/a/../b", "b.md"}, // path.Clean resolves ".." against the preceding segment
		{"https://example.com/weird%20name", "weird-name.md"},
		{"https://example.com/café", "café.md"},
	}
	for _, tc := range cases {
		got := SlugForURL(mustURL(t, tc.in))
		if got != tc.want {
			t.Errorf("SlugForURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugForURL_NoPathTraversalEscape(t *testing.T) {
	u := mustURL(t, "https://example.com/../../etc/passwd")
	got := SlugForURL(u)
	if strings.Contains(got, "..") {
		t.Errorf("SlugForURL(%q) = %q, contains path traversal", u, got)
	}
}

func TestWritePage(t *testing.T) {
	dir := t.TempDir()
	page := &content.Page{
		URL:         "https://example.com/products/shoes",
		Title:       "Blue Shoes",
		Description: "Nice shoes",
		Lang:        "en",
		CrawledAt:   time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
		Hash:        "abc123",
		Headings:    []content.Heading{{Level: 1, Text: "Blue Shoes"}},
		Markdown:    "# Blue Shoes\n\nGreat shoes for running.",
	}

	rel, err := WritePage(dir, mustURL(t, page.URL), page)
	if err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if rel != "products/shoes.md" {
		t.Errorf("rel = %q, want products/shoes.md", rel)
	}

	data, err := os.ReadFile(filepath.Join(dir, "products", "shoes.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)

	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("expected frontmatter delimiter at start, got: %q", got[:40])
	}
	if !strings.Contains(got, "title: Blue Shoes") {
		t.Errorf("missing title in frontmatter: %q", got)
	}
	if !strings.Contains(got, "url: https://example.com/products/shoes") {
		t.Errorf("missing url in frontmatter: %q", got)
	}
	if !strings.Contains(got, `crawled_at: "2026-08-24T10:00:00Z"`) {
		t.Errorf("missing/wrong crawled_at: %q", got)
	}
	if !strings.Contains(got, "# Blue Shoes\n\nGreat shoes for running.") {
		t.Errorf("missing markdown body: %q", got)
	}
}

func TestWritePage_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	page := &content.Page{URL: "https://example.com/a/b/c", Title: "Deep", Markdown: "hi"}
	rel, err := WritePage(dir, mustURL(t, page.URL), page)
	if err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}
