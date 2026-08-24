// Package export emits an indexed site's knowledge base as markdown files
// (M2) or JSONL (M3), and writes the per-page markdown files the crawler
// produces into <data_dir>/<site>/kb/.
//
// See CLAUDE.md, "Command Surface (CLI)" and "Data dir" layout.
package export

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/Xara-AI/sitedex/internal/extract/content"
)

// frontmatter mirrors the YAML header CLAUDE.md documents for exported
// markdown pages.
type frontmatter struct {
	URL         string            `yaml:"url"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description,omitempty"`
	Lang        string            `yaml:"lang,omitempty"`
	CrawledAt   string            `yaml:"crawled_at"`
	Hash        string            `yaml:"hash"`
	Headings    []content.Heading `yaml:"headings,omitempty"`
}

// SlugForURL derives a filesystem-safe, human-legible relative path (no
// leading slash, ".md" suffix) for a page URL, e.g.
// "https://example.com/products/blue-shoes?x=1" -> "products/blue-shoes.md".
// The root path maps to "index.md". Query strings are intentionally
// ignored: pages that differ only by query parameter collide onto the same
// file in v1, which is an accepted simplification (see CLAUDE.md's
// out-of-scope notes on this being revisited if it proves painful).
func SlugForURL(u *url.URL) string {
	// path.Clean resolves "." and ".." segments properly (rather than just
	// dropping them) and never escapes above root, which also makes this
	// safe against path-traversal attempts in a crawled URL's path.
	p := strings.TrimPrefix(path.Clean("/"+u.Path), "/")
	if p == "" || p == "." {
		return "index.md"
	}

	segs := strings.Split(p, "/")
	clean := make([]string, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		clean = append(clean, sanitizeSegment(s))
	}
	if len(clean) == 0 {
		return "index.md"
	}
	return path.Join(clean...) + ".md"
}

// sanitizeSegment makes one URL path segment safe as a filename on every
// target platform (Windows forbids `< > : " / \ | ? *` and control
// characters). Unicode letters/digits are kept as-is — Romanian diacritics
// and similar are legitimate, meaningful parts of a slug, not noise to
// strip.
func sanitizeSegment(s string) string {
	if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			// Whitespace, path separators, and Windows-invalid punctuation
			// all collapse to a hyphen.
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "page"
	}
	return out
}

// WritePage renders page as markdown with YAML frontmatter and writes it
// to <kbDir>/<SlugForURL(pageURL)>, creating parent directories as needed.
// It returns the path written, relative to kbDir.
func WritePage(kbDir string, pageURL *url.URL, page *content.Page) (relPath string, err error) {
	rel := SlugForURL(pageURL)
	full := filepath.Join(kbDir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("create kb dir: %w", err)
	}

	fm := frontmatter{
		URL:         page.URL,
		Title:       page.Title,
		Description: page.Description,
		Lang:        page.Lang,
		CrawledAt:   page.CrawledAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Hash:        page.Hash,
		Headings:    page.Headings,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	var doc strings.Builder
	doc.WriteString("---\n")
	doc.Write(fmBytes)
	doc.WriteString("---\n\n")
	doc.WriteString(page.Markdown)
	doc.WriteString("\n")

	if err := os.WriteFile(full, []byte(doc.String()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", full, err)
	}
	return rel, nil
}
