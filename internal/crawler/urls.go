package crawler

import (
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// NormalizeURL canonicalizes u for dedup purposes: lowercases scheme and
// host, strips the fragment, drops default ports, and removes a trailing
// slash on non-root paths.
func NormalizeURL(u *url.URL) *url.URL {
	n := *u
	n.Scheme = strings.ToLower(n.Scheme)
	n.Host = strings.ToLower(n.Host)
	n.Fragment = ""
	n.RawFragment = ""

	if (n.Scheme == "http" && strings.HasSuffix(n.Host, ":80")) ||
		(n.Scheme == "https" && strings.HasSuffix(n.Host, ":443")) {
		if i := strings.LastIndex(n.Host, ":"); i >= 0 {
			n.Host = n.Host[:i]
		}
	}

	if n.Path == "" {
		n.Path = "/"
	} else if len(n.Path) > 1 && strings.HasSuffix(n.Path, "/") {
		n.Path = strings.TrimSuffix(n.Path, "/")
	}

	return &n
}

// ExtractLinks parses raw (an HTML document) and returns every <a href>
// target, resolved against base and normalized. It intentionally looks at
// the whole document, including nav/footer chrome that content extraction
// will later strip, since site navigation is exactly how the crawler
// discovers most pages.
func ExtractLinks(base *url.URL, raw []byte) []*url.URL {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var out []*url.URL

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key != "href" {
					continue
				}
				href := strings.TrimSpace(attr.Val)
				if href == "" || strings.HasPrefix(href, "#") ||
					strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") ||
					strings.HasPrefix(href, "tel:") {
					continue
				}
				ref, err := url.Parse(href)
				if err != nil {
					continue
				}
				abs := NormalizeURL(base.ResolveReference(ref))
				if abs.Scheme != "http" && abs.Scheme != "https" {
					continue
				}
				key := abs.String()
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, abs)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
