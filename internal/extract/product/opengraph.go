package product

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// extractOpenGraph handles the OpenGraph product extension
// (og:type=product plus the product:* namespace Meta's commerce tags
// popularized, which many platforms emit even without full JSON-LD).
func extractOpenGraph(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	ogType := metaProperty(doc, "og:type")
	if !strings.EqualFold(ogType, "product") && !strings.EqualFold(ogType, "product.item") {
		return nil, false
	}

	p := &Product{ExtractionMethod: MethodOpenGraph, Availability: Unknown}
	p.Name = normalizeSpace(firstNonEmpty(metaProperty(doc, "og:title"), metaProperty(doc, "product:title")))
	p.Description = normalizeSpace(metaProperty(doc, "og:description"))
	if img := metaProperty(doc, "og:image"); img != "" {
		p.Image = resolveURL(pageURL, img)
	}

	priceStr := firstNonEmpty(metaProperty(doc, "product:price:amount"), metaProperty(doc, "og:price:amount"))
	if price, ok := priceField(priceStr); ok {
		p.Price, p.HasPrice = price, true
	}
	p.Currency = firstNonEmpty(metaProperty(doc, "product:price:currency"), metaProperty(doc, "og:price:currency"))
	p.Availability = availabilityField(metaProperty(doc, "product:availability"))

	if p.Name == "" {
		return nil, false
	}
	p.URL = pageURL.String()
	return p, true
}
