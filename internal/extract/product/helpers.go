package product

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

var wsRE = regexp.MustCompile(`\s+`)

func normalizeSpace(s string) string {
	return strings.TrimSpace(wsRE.ReplaceAllString(s, " "))
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return normalizeSpace(sb.String())
}

func findByTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func textOfTag(n *html.Node, tag string) string {
	found := findByTag(n, tag)
	if found == nil {
		return ""
	}
	return textContent(found)
}

func findByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && attrVal(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attrVal(n, "class")) {
		if strings.EqualFold(c, class) {
			return true
		}
	}
	return false
}

func findByClass(n *html.Node, class string) *html.Node {
	if n.Type == html.ElementNode && hasClass(n, class) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByClass(c, class); found != nil {
			return found
		}
	}
	return nil
}

func findByAnyClass(n *html.Node, classes ...string) *html.Node {
	for _, c := range classes {
		if found := findByClass(n, c); found != nil {
			return found
		}
	}
	return nil
}

func textOfClass(n *html.Node, class string) string {
	found := findByClass(n, class)
	if found == nil {
		return ""
	}
	return textContent(found)
}

func textOfAnyClass(n *html.Node, classes ...string) string {
	found := findByAnyClass(n, classes...)
	if found == nil {
		return ""
	}
	return textContent(found)
}

func srcOfAnyClass(n *html.Node, classes ...string) string {
	found := findByAnyClass(n, classes...)
	if found == nil {
		return ""
	}
	if found.Data == "img" {
		return attrVal(found, "src")
	}
	if img := findByTag(found, "img"); img != nil {
		return attrVal(img, "src")
	}
	return ""
}

func metaContent(doc *html.Node, attrKey, want string) string {
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

func metaProperty(doc *html.Node, property string) string {
	return metaContent(doc, "property", property)
}

func metaName(doc *html.Node, name string) string {
	return metaContent(doc, "name", name)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if base == nil || ref == "" {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}

// priceField tolerantly extracts a price from a JSON-decoded value (a
// float64 from numeric JSON, or a string like "19.99", "1,299.00", or
// "1.299,00") or a plain string (from HTML attribute/text extraction).
func priceField(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		return parsePriceString(t)
	}
	return 0, false
}

func parsePriceString(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) || r == '.' || r == ',' || r == '-' {
			b.WriteRune(r)
		}
	}
	cleaned := normalizeDecimalSeparator(b.String())
	if cleaned == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// normalizeDecimalSeparator handles both "1,234.56" (US) and "1.234,56"
// (EU) styles by treating whichever of ','/'.' appears last as the decimal
// point and stripping the other as a thousands separator.
func normalizeDecimalSeparator(s string) string {
	lastComma := strings.LastIndexByte(s, ',')
	lastDot := strings.LastIndexByte(s, '.')
	if lastComma == -1 {
		return strings.ReplaceAll(s, ",", "")
	}
	if lastComma > lastDot {
		s = strings.ReplaceAll(s, ".", "")
		return strings.Replace(s, ",", ".", 1)
	}
	return strings.ReplaceAll(s, ",", "")
}

// currencyFromSymbol makes a best-effort currency guess from a price
// string's symbol — useful when a platform's markup shows only "$19.99"
// with no separate machine-readable currency field.
func currencyFromSymbol(s string) string {
	switch {
	case strings.Contains(s, "$"):
		return "USD"
	case strings.Contains(s, "€"):
		return "EUR"
	case strings.Contains(s, "£"):
		return "GBP"
	case strings.Contains(strings.ToLower(s), "lei"), strings.Contains(s, "RON"):
		return "RON"
	}
	return ""
}

// availabilityField normalizes the many vocabularies platforms use
// ("https://schema.org/InStock", "in stock", "oos", "sold out", ...) onto
// InStock/OutOfStock/Unknown.
func availabilityField(v interface{}) Availability {
	s, _ := v.(string)
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return Unknown
	case strings.Contains(s, "outofstock"), strings.Contains(s, "out of stock"),
		strings.Contains(s, "soldout"), strings.Contains(s, "sold out"),
		s == "oos", strings.Contains(s, "discontinued"):
		return OutOfStock
	case strings.Contains(s, "instock"), strings.Contains(s, "in stock"),
		strings.Contains(s, "limitedavailability"), strings.Contains(s, "limited availability"),
		strings.Contains(s, "available for order"), strings.Contains(s, "preorder"),
		strings.Contains(s, "presale"):
		return InStock
	default:
		return Unknown
	}
}
