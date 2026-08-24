package product

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// extractJSONLD scans every <script type="application/ld+json"> block for
// a Product node, tolerantly: a block may be a single object, an array of
// objects, or wrap its content in "@graph"; a malformed block is skipped
// rather than aborting the whole page.
func extractJSONLD(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	for _, raw := range jsonLDScripts(doc) {
		var v interface{}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			continue
		}
		candidates := findProductCandidates(v)
		if len(candidates) == 0 {
			continue
		}
		p := productFromJSONLD(candidates[0], pageURL)
		if p.Name == "" {
			continue // not enough to call this a successful extraction
		}
		p.URL = pageURL.String()
		p.RawJSON = raw
		return p, true
	}
	return nil, false
}

func jsonLDScripts(doc *html.Node) []string {
	var scripts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" &&
			strings.EqualFold(strings.TrimSpace(attrVal(n, "type")), "application/ld+json") {
			scripts = append(scripts, rawText(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return scripts
}

func rawText(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

// findProductCandidates recursively walks a decoded JSON-LD tree (which
// may nest Products inside "@graph", "itemListElement", or arbitrarily
// deep object/array structure) and returns every object whose "@type"
// includes "Product", in a deterministic pre-order (map keys are visited
// in sorted order, since Go's JSON decoding into map[string]interface{}
// does not preserve source key order).
func findProductCandidates(v interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	var walk func(interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			if hasLDType(t, "Product") {
				out = append(out, t)
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k])
			}
		case []interface{}:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(v)
	return out
}

func hasLDType(m map[string]interface{}, want string) bool {
	switch t := m["@type"].(type) {
	case string:
		return strings.EqualFold(t, want)
	case []interface{}:
		for _, x := range t {
			if s, ok := x.(string); ok && strings.EqualFold(s, want) {
				return true
			}
		}
	}
	return false
}

func productFromJSONLD(m map[string]interface{}, pageURL *url.URL) *Product {
	p := &Product{ExtractionMethod: MethodJSONLD, Availability: Unknown}

	if s, ok := m["name"].(string); ok {
		p.Name = normalizeSpace(s)
	}
	if s, ok := m["description"].(string); ok {
		p.Description = normalizeSpace(s)
	}
	if img := ldImage(m["image"]); img != "" {
		p.Image = resolveURL(pageURL, img)
	}

	if offer := ldOffer(m["offers"]); offer != nil {
		if price, ok := priceField(offer["price"]); ok {
			p.Price, p.HasPrice = price, true
		}
		if s, ok := offer["priceCurrency"].(string); ok {
			p.Currency = s
		}
		p.Availability = availabilityField(offer["availability"])
	}
	if !p.HasPrice {
		if price, ok := priceField(m["price"]); ok {
			p.Price, p.HasPrice = price, true
		}
	}

	return p
}

// ldImage handles image being a bare URL string, an array of them (first
// wins), or an ImageObject with a "url" field.
func ldImage(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		for _, item := range t {
			if s := ldImage(item); s != "" {
				return s
			}
		}
	case map[string]interface{}:
		if u, ok := t["url"].(string); ok {
			return u
		}
	}
	return ""
}

// ldOffer normalizes "offers" (a single Offer, an array of them — first
// wins, or an AggregateOffer, whose lowPrice stands in for price) into a
// plain field map.
func ldOffer(v interface{}) map[string]interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		if hasLDType(t, "AggregateOffer") {
			norm := map[string]interface{}{}
			if lp, ok := t["lowPrice"]; ok {
				norm["price"] = lp
			} else if p, ok := t["price"]; ok {
				norm["price"] = p
			}
			if c, ok := t["priceCurrency"]; ok {
				norm["priceCurrency"] = c
			}
			if a, ok := t["availability"]; ok {
				norm["availability"] = a
			}
			return norm
		}
		return t
	case []interface{}:
		for _, item := range t {
			if m, ok := item.(map[string]interface{}); ok {
				return m
			}
		}
	}
	return nil
}
