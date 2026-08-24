package product

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// extractMicrodata handles the common two-level schema.org microdata
// shape: an itemscope itemtype=".../Product" containing itemprops, one of
// which ("offers") is itself a nested itemscope itemtype=".../Offer" (or
// AggregateOffer). It is not a general microdata/RDFa parser — just enough
// to cover the Product/Offer pattern platforms actually emit.
func extractMicrodata(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	root := findItemscope(doc, "product")
	if root == nil {
		return nil, false
	}
	props := scopedItemProps(root)

	p := &Product{ExtractionMethod: MethodMicrodata, Availability: Unknown}
	p.Name = normalizeSpace(firstItemPropValue(props, "name"))
	p.Description = normalizeSpace(firstItemPropValue(props, "description"))
	if img := firstItemPropValue(props, "image"); img != "" {
		p.Image = resolveURL(pageURL, img)
	}

	offerProps := props
	if offerNodes, ok := props["offers"]; ok && len(offerNodes) > 0 && hasAttr(offerNodes[0], "itemscope") {
		offerProps = scopedItemProps(offerNodes[0])
	}
	if price, ok := priceField(firstItemPropValue(offerProps, "price")); ok {
		p.Price, p.HasPrice = price, true
	}
	p.Currency = firstItemPropValue(offerProps, "priceCurrency")
	p.Availability = availabilityField(firstItemPropValue(offerProps, "availability"))

	if p.Name == "" {
		return nil, false
	}
	p.URL = pageURL.String()
	return p, true
}

// findItemscope finds the first element with an itemscope attribute whose
// itemtype references schema.org/<wantType> (any scheme, case-insensitive).
func findItemscope(n *html.Node, wantType string) *html.Node {
	if n.Type == html.ElementNode && hasAttr(n, "itemscope") {
		it := strings.ToLower(attrVal(n, "itemtype"))
		if strings.Contains(it, "schema.org/"+strings.ToLower(wantType)) {
			return n
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findItemscope(c, wantType); found != nil {
			return found
		}
	}
	return nil
}

// scopedItemProps collects itemprop name -> declaring node(s) for props
// belonging to scope, stopping descent at any nested itemscope (per
// microdata scoping rules, its properties belong to that nested item, not
// this one) — though the nested element itself is still recorded under
// its own itemprop name, e.g. "offers", so callers can recurse into it.
func scopedItemProps(scope *html.Node) map[string][]*html.Node {
	props := make(map[string][]*html.Node)
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, isScopeRoot bool) {
		if n.Type == html.ElementNode && !isScopeRoot {
			if name := attrVal(n, "itemprop"); name != "" {
				props[name] = append(props[name], n)
			}
			if hasAttr(n, "itemscope") {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}
	walk(scope, true)
	return props
}

func firstItemPropValue(props map[string][]*html.Node, name string) string {
	nodes := props[name]
	if len(nodes) == 0 {
		return ""
	}
	return itemPropValue(nodes[0])
}

// itemPropValue reads an itemprop's value per the microdata spec's
// per-element rules: a meta/link/a/img/time element's machine-readable
// attribute wins over its text content.
func itemPropValue(n *html.Node) string {
	switch n.Data {
	case "meta":
		return attrVal(n, "content")
	case "link", "a":
		if v := attrVal(n, "href"); v != "" {
			return v
		}
	case "img", "source":
		if v := attrVal(n, "src"); v != "" {
			return v
		}
	case "time":
		if v := attrVal(n, "datetime"); v != "" {
			return v
		}
	}
	if v := attrVal(n, "content"); v != "" {
		return v
	}
	return textContent(n)
}
