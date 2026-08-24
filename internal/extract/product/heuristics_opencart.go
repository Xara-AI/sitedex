package product

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// openCartDetector recognizes OpenCart by its generator meta tag, then
// reads the default theme's "#content" area, where the product title is a
// plain <h1> and price/stock live in ".price"/".stock" elements.
type openCartDetector struct{}

func (openCartDetector) Name() string { return "opencart" }

func (openCartDetector) Detect(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	if !strings.Contains(strings.ToLower(metaName(doc, "generator")), "opencart") {
		return nil, false
	}

	scope := findByID(doc, "content")
	if scope == nil {
		scope = doc
	}

	name := textOfTag(scope, "h1")
	if name == "" {
		return nil, false
	}

	priceText := textOfAnyClass(scope, "price-new", "price")
	price, hasPrice := priceField(priceText)

	availability := Unknown
	stockText := strings.ToLower(textOfAnyClass(scope, "stock"))
	switch {
	case strings.Contains(stockText, "in stock"):
		availability = InStock
	case strings.Contains(stockText, "out of stock"), strings.Contains(stockText, "unavailable"):
		availability = OutOfStock
	}

	img := srcOfAnyClass(scope, "img-thumbnail")
	if img == "" {
		if imgNode := findByTag(scope, "img"); imgNode != nil {
			img = attrVal(imgNode, "src")
		}
	}

	return &Product{
		URL: pageURL.String(), Name: normalizeSpace(name),
		Price: price, HasPrice: hasPrice, Currency: currencyFromSymbol(priceText),
		Availability: availability, Image: resolveURL(pageURL, img),
		ExtractionMethod: MethodHeuristic,
	}, true
}
