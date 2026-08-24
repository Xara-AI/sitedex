package product

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// prestaShopDetector recognizes PrestaShop by its generator meta tag (most
// installs keep the default), then reads the Classic theme's price and
// availability class names. In practice PrestaShop's default theme also
// emits schema.org microdata, so this heuristic mainly matters for
// customized themes that dropped it.
type prestaShopDetector struct{}

func (prestaShopDetector) Name() string { return "prestashop" }

func (prestaShopDetector) Detect(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	if !strings.Contains(strings.ToLower(metaName(doc, "generator")), "prestashop") {
		return nil, false
	}

	name := textOfAnyClass(doc, "product-title")
	if name == "" {
		name = textOfTag(doc, "h1")
	}
	if name == "" {
		return nil, false
	}

	priceText := textOfAnyClass(doc, "current-price", "product-price")
	price, hasPrice := priceField(priceText)

	availability := Unknown
	switch {
	case findByAnyClass(doc, "product-available", "available") != nil:
		availability = InStock
	case findByAnyClass(doc, "product-unavailable", "unavailable") != nil:
		availability = OutOfStock
	}

	img := srcOfAnyClass(doc, "product-cover", "js-qv-product-cover")

	return &Product{
		URL: pageURL.String(), Name: normalizeSpace(name),
		Price: price, HasPrice: hasPrice, Currency: currencyFromSymbol(priceText),
		Availability: availability, Image: resolveURL(pageURL, img),
		ExtractionMethod: MethodHeuristic,
	}, true
}
