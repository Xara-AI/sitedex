package product

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// shopifyDetector recognizes Shopify storefronts by the presence of a
// script/link tag loaded from Shopify's CDN — a reliable signal
// independent of theme — then reads common Dawn/legacy-theme class names
// for the product title, price, and image.
type shopifyDetector struct{}

func (shopifyDetector) Name() string { return "shopify" }

func (shopifyDetector) Detect(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	if !hasShopifyMarker(doc) {
		return nil, false
	}

	name := textOfAnyClass(doc, "product__title", "product-single__title", "product-meta__title")
	if name == "" {
		return nil, false
	}

	priceText := textOfAnyClass(doc, "price-item--regular", "product__price", "price__regular", "price-item")
	price, hasPrice := priceField(priceText)

	availability := Unknown
	if hasPrice {
		availability = InStock
	}
	if findByAnyClass(doc, "badge--sold-out", "product__price--sold-out") != nil {
		availability = OutOfStock
	}

	img := srcOfAnyClass(doc, "product__media", "product-single__photo", "product__image")

	return &Product{
		URL: pageURL.String(), Name: normalizeSpace(name),
		Price: price, HasPrice: hasPrice, Currency: currencyFromSymbol(priceText),
		Availability: availability, Image: resolveURL(pageURL, img),
		ExtractionMethod: MethodHeuristic,
	}, true
}

func hasShopifyMarker(doc *html.Node) bool {
	found := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "link") {
			src := attrVal(n, "src") + attrVal(n, "href")
			if strings.Contains(src, "cdn.shopify.com") || strings.Contains(src, "shopifycdn.com") {
				found = true
				return
			}
		}
		for c := n.FirstChild; c != nil && !found; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}
