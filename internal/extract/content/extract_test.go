package content

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestExtract_TitleFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"title tag wins", `<html><head><title> My Page </title><meta property="og:title" content="OG Title"></head><body><main><h1>H1</h1></main></body></html>`, "My Page"},
		{"og:title fallback", `<html><head><meta property="og:title" content="OG Title"></head><body><main><p>text long enough to count as content for density scoring purposes here yes indeed</p></main></body></html>`, "OG Title"},
		{"h1 fallback", `<html><body><main><h1>Heading One</h1><p>text long enough to count as content for density scoring purposes here yes indeed</p></main></body></html>`, "Heading One"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Extract([]byte(tc.html), mustURL(t, "https://example.com/"), time.Now())
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if p.Title != tc.want {
				t.Errorf("Title = %q, want %q", p.Title, tc.want)
			}
		})
	}
}

func TestExtract_DescriptionAndLang(t *testing.T) {
	html := `<html lang="ro"><head>
<meta name="description" content="Meta desc">
</head><body><main><p>Some real page content that is long enough to be picked as the main content block by density scoring.</p></main></body></html>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.Description != "Meta desc" {
		t.Errorf("Description = %q, want Meta desc", p.Description)
	}
	if p.Lang != "ro" {
		t.Errorf("Lang = %q, want ro", p.Lang)
	}
}

func TestExtract_StripsBoilerplate(t *testing.T) {
	html := `<html><body>
<nav><a href="/">Home</a><a href="/x">X</a></nav>
<header class="site-header">Logo here</header>
<div id="cookie-banner">We use cookies to enhance your experience, click accept to continue browsing our site.</div>
<main>
  <h1>Article Title</h1>
  <p>This is the real article content that should survive extraction intact and be present in the output markdown, with enough length to clear the content-density threshold reliably.</p>
</main>
<footer>Copyright 2026 Example Corp. All rights reserved.</footer>
</body></html>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Markdown, "cookies to enhance") {
		t.Error("cookie banner text leaked into markdown")
	}
	if strings.Contains(p.Markdown, "Copyright 2026") {
		t.Error("footer text leaked into markdown")
	}
	if strings.Contains(p.Markdown, "Home") || strings.Contains(p.Markdown, "Logo here") {
		t.Error("nav/header text leaked into markdown")
	}
	if !strings.Contains(p.Markdown, "This is the real article content") {
		t.Errorf("expected real content in markdown, got: %q", p.Markdown)
	}
}

func TestExtract_PrefersMainOverDensityCandidate(t *testing.T) {
	html := `<html><body>
<div class="promo-links">` + strings.Repeat(`<a href="/x">link text padding to inflate size </a>`, 20) + `</div>
<main><p>This is the real main content block, and it needs to be long enough to clear the minimum content length threshold used when selecting the primary content element in this test.</p></main>
</body></html>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Markdown, "real main content block") {
		t.Errorf("expected <main> content to win over a large link farm, got: %q", p.Markdown)
	}
	if strings.Contains(p.Markdown, "link text padding") {
		t.Error("link-farm div should not have been selected as content")
	}
}

func TestExtract_HeadingsOutlineAndLinks(t *testing.T) {
	html := `<main>
<h1>Top</h1>
<p>intro <a href="/rel/path">relative link</a> and <a href="https://other.com/x">absolute link</a>, with some extra surrounding prose to pad this paragraph out a bit further.</p>
<h2>Sub</h2>
<p>more content here to satisfy density thresholds nicely and completely, plus a little more padding text for good measure.</p>
</main>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/base/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Headings) != 2 || p.Headings[0].Text != "Top" || p.Headings[0].Level != 1 ||
		p.Headings[1].Text != "Sub" || p.Headings[1].Level != 2 {
		t.Errorf("Headings = %+v", p.Headings)
	}
	if !strings.Contains(p.Markdown, "[relative link](https://example.com/rel/path)") {
		t.Errorf("relative link not resolved correctly, markdown: %q", p.Markdown)
	}
	if !strings.Contains(p.Markdown, "[absolute link](https://other.com/x)") {
		t.Errorf("absolute link mangled, markdown: %q", p.Markdown)
	}
}

func TestExtract_ListsBlockquotePreTable(t *testing.T) {
	html := `<main>
<ul>
<li>First item</li>
<li>Second item
  <ul><li>Nested item</li></ul>
</li>
</ul>
<blockquote>A quoted line of reasonable length for testing purposes.</blockquote>
<pre>line one
line two</pre>
<table>
<tr><th>Name</th><th>Price</th></tr>
<tr><td>Widget</td><td>9.99</td></tr>
</table>
<p>padding text so this main element clears the density/length threshold nicely.</p>
</main>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	md := p.Markdown
	if !strings.Contains(md, "- First item") {
		t.Errorf("missing list item, markdown: %q", md)
	}
	if !strings.Contains(md, "  - Nested item") {
		t.Errorf("missing nested list item, markdown: %q", md)
	}
	if !strings.Contains(md, "> A quoted line") {
		t.Errorf("missing blockquote, markdown: %q", md)
	}
	if !strings.Contains(md, "```\nline one\nline two\n```") {
		t.Errorf("missing code block, markdown: %q", md)
	}
	if !strings.Contains(md, "| Name | Price |") || !strings.Contains(md, "| Widget | 9.99 |") {
		t.Errorf("missing table, markdown: %q", md)
	}
}

func TestExtract_HashStableAcrossInsignificantWhitespace(t *testing.T) {
	a := `<main><p>Hello    world,   this is a stable   piece of content for hashing purposes right here.</p></main>`
	b := `<main>
<p>Hello world,
   this is a stable piece of content for hashing purposes right here.</p>
</main>`
	pa, err := Extract([]byte(a), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pb, err := Extract([]byte(b), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if pa.Hash != pb.Hash {
		t.Errorf("hashes differ despite equivalent content: %q vs %q", pa.Hash, pb.Hash)
	}
}

func TestExtract_HashChangesWithContent(t *testing.T) {
	a := `<main><p>Original content that is long enough to pass the density threshold check.</p></main>`
	b := `<main><p>Changed content that is long enough to pass the density threshold check.</p></main>`
	pa, _ := Extract([]byte(a), mustURL(t, "https://example.com/"), time.Now())
	pb, _ := Extract([]byte(b), mustURL(t, "https://example.com/"), time.Now())
	if pa.Hash == pb.Hash {
		t.Error("expected different hashes for different content")
	}
}

func TestExtract_ImagesRenderWithResolvedSrc(t *testing.T) {
	html := `<main><p>intro text long enough to pass the density threshold for main content here.</p><img src="/img/photo.jpg" alt="A photo"></main>`
	p, err := Extract([]byte(html), mustURL(t, "https://example.com/"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Markdown, "![A photo](https://example.com/img/photo.jpg)") {
		t.Errorf("expected resolved image markdown, got: %q", p.Markdown)
	}
}
