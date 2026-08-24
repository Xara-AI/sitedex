package content

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Extract converts raw HTML into a Page: title/description/lang metadata,
// clean markdown with boilerplate stripped, a heading outline, and a
// content hash (sha256 of the markdown body) for change detection across
// re-crawls.
func Extract(raw []byte, pageURL *url.URL, crawledAt time.Time) (*Page, error) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}

	title := metaTitle(doc)
	description := metaDescription(doc)
	lang := metaLang(doc)

	stripBoilerplate(doc)
	contentNode := selectContentNode(doc)
	markdown, headings := renderMarkdown(contentNode, pageURL)

	hash := sha256.Sum256([]byte(markdown))

	return &Page{
		URL:         pageURL.String(),
		Title:       title,
		Description: description,
		Lang:        lang,
		CrawledAt:   crawledAt,
		Hash:        hex.EncodeToString(hash[:]),
		Headings:    headings,
		Markdown:    markdown,
	}, nil
}

func metaTitle(doc *html.Node) string {
	if t := findFirst(doc, "title"); t != nil {
		if s := textContent(t); s != "" {
			return s
		}
	}
	if s := metaProperty(doc, "og:title"); s != "" {
		return s
	}
	if h1 := findFirst(doc, "h1"); h1 != nil {
		return textContent(h1)
	}
	return ""
}

func metaDescription(doc *html.Node) string {
	if s := metaName(doc, "description"); s != "" {
		return s
	}
	if s := metaProperty(doc, "og:description"); s != "" {
		return s
	}
	return ""
}

func metaLang(doc *html.Node) string {
	if h := findFirst(doc, "html"); h != nil {
		if l := attrVal(h, "lang"); l != "" {
			return l
		}
	}
	return ""
}

func metaName(doc *html.Node, name string) string {
	return findMetaContent(doc, "name", name)
}

func metaProperty(doc *html.Node, property string) string {
	return findMetaContent(doc, "property", property)
}

// findMetaContent finds the first <meta attrKey="want" content="..."> and
// returns its content, matching attrKey's value case-insensitively.
func findMetaContent(doc *html.Node, attrKey, want string) string {
	var result string
	var found bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			if strings.EqualFold(attrVal(n, attrKey), want) {
				result = strings.TrimSpace(attrVal(n, "content"))
				found = true
				return
			}
		}
		for c := n.FirstChild; c != nil && !found; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return result
}
