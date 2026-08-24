package content

import (
	"strings"

	"golang.org/x/net/html"
)

// boilerplateTags are always dropped regardless of context: they're never
// page content.
var boilerplateTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true,
	"svg": true, "nav": true, "footer": true, "aside": true, "form": true,
	"button": true, "select": true, "textarea": true, "input": true,
	"template": true, "object": true, "embed": true,
}

// boilerplateHints are substrings checked (case-insensitively) against an
// element's class/id; a match drops the whole subtree. This is a
// deliberately small, extensible heuristic list, not an attempt at
// completeness — see CLAUDE.md's content-extraction section.
var boilerplateHints = []string{
	"cookie", "consent", "gdpr", "banner", "advert", "sidebar", "popup",
	"modal", "newsletter", "subscribe", "breadcrumb", "social-share",
	"share-buttons", "site-header", "site-footer", "skip-link",
	"back-to-top", "site-nav",
}

// stripBoilerplate removes boilerplate subtrees from doc in place: chrome
// tags (nav/footer/aside/...), non-content tags (script/style/...), and
// elements whose class/id hints at cookie banners, ads, or similar.
func stripBoilerplate(doc *html.Node) {
	var toRemove []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if boilerplateTags[n.Data] || hasBoilerplateHint(n) {
				toRemove = append(toRemove, n)
				return // don't descend into a subtree we're about to drop
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
}

func hasBoilerplateHint(n *html.Node) bool {
	class := strings.ToLower(attrVal(n, "class"))
	id := strings.ToLower(attrVal(n, "id"))
	for _, kw := range boilerplateHints {
		if strings.Contains(class, kw) || strings.Contains(id, kw) {
			return true
		}
	}
	return false
}

// minContentChars is the minimum text length a candidate content node
// needs before it's trusted as "the" content, rather than an empty or
// near-empty <main>/<article> shell.
const minContentChars = 140

// selectContentNode picks the DOM subtree to render as the page's content,
// after stripBoilerplate has already run. It prefers semantic tags
// (<main>, then <article>), falling back to a text-density scoring pass
// over <div>/<section> candidates, and finally to <body> itself.
func selectContentNode(doc *html.Node) *html.Node {
	if m := findFirst(doc, "main"); m != nil && len(textContent(m)) >= minContentChars {
		return m
	}
	if a := findFirst(doc, "article"); a != nil && len(textContent(a)) >= minContentChars {
		return a
	}
	if best := findByDensity(doc); best != nil {
		return best
	}
	if body := findFirst(doc, "body"); body != nil {
		return body
	}
	return doc
}

// findByDensity scores every div/section/article/main candidate by
// text length weighted down by link density (a proxy for "this is a link
// farm / menu, not an article"), and returns the highest scorer.
func findByDensity(doc *html.Node) *html.Node {
	var best *html.Node
	bestScore := 0.0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "div", "section", "article", "main":
				text := textContent(n)
				if len(text) >= minContentChars {
					link := linkTextContent(n)
					density := 0.0
					if len(text) > 0 {
						density = float64(len(link)) / float64(len(text))
					}
					score := float64(len(text)) * (1 - density)
					if score > bestScore {
						bestScore = score
						best = n
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return best
}
