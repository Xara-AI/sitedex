package product

import (
	"net/url"

	"golang.org/x/net/html"
)

// wooCommerceDetector recognizes WooCommerce's default theme markup:
// ".product" pages under a ".woocommerce" wrapper, with a
// ".woocommerce-Price-amount" price and "in-stock"/"out-of-stock" status
// classes. The ".woocommerce" wrapper check exists specifically so a
// generic ".product" div on some other platform's page doesn't false-
// positive here.
type wooCommerceDetector struct{}

func (wooCommerceDetector) Name() string { return "woocommerce" }

func (wooCommerceDetector) Detect(doc *html.Node, pageURL *url.URL) (*Product, bool) {
	if findByClass(doc, "woocommerce") == nil {
		return nil, false
	}
	root := findByClass(doc, "product")
	if root == nil {
		root = doc
	}

	name := textOfAnyClass(root, "product_title", "entry-title")
	if name == "" {
		return nil, false
	}

	priceText := textOfClass(root, "woocommerce-Price-amount")
	price, hasPrice := priceField(priceText)

	availability := Unknown
	switch {
	case findByClass(root, "in-stock") != nil:
		availability = InStock
	case findByClass(root, "out-of-stock") != nil:
		availability = OutOfStock
	}

	img := srcOfAnyClass(root, "wp-post-image", "woocommerce-product-gallery__image")

	return &Product{
		URL: pageURL.String(), Name: normalizeSpace(name),
		Price: price, HasPrice: hasPrice, Currency: currencyFromSymbol(priceText),
		Availability: availability, Image: resolveURL(pageURL, img),
		ExtractionMethod: MethodHeuristic,
	}, true
}
