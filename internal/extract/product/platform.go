package product

import (
	"strings"

	"golang.org/x/net/html"
)

// Platform is a recognized e-commerce platform, identified from a page's
// markup. It's shared between the CSS-heuristic product detectors (which
// use it to avoid false-positiving on generic markup — see
// heuristics_*.go) and the cold-path site-search endpoint detection in
// internal/search (which uses it to pick a platform's known search URL
// pattern before falling back to generic <form role="search"> discovery).
type Platform string

const (
	PlatformWooCommerce Platform = "woocommerce"
	PlatformShopify     Platform = "shopify"
	PlatformPrestaShop  Platform = "prestashop"
	PlatformOpenCart    Platform = "opencart"
	PlatformUnknown     Platform = ""
)

// DetectPlatform identifies which (if any) of the platforms sitedex knows
// about produced doc, using the same markers each platform's Detector
// checks for.
func DetectPlatform(doc *html.Node) Platform {
	switch {
	case hasWooCommerceMarker(doc):
		return PlatformWooCommerce
	case hasShopifyMarker(doc):
		return PlatformShopify
	case hasGeneratorMarker(doc, "prestashop"):
		return PlatformPrestaShop
	case hasGeneratorMarker(doc, "opencart"):
		return PlatformOpenCart
	default:
		return PlatformUnknown
	}
}

func hasWooCommerceMarker(doc *html.Node) bool {
	return findByClass(doc, "woocommerce") != nil
}

func hasGeneratorMarker(doc *html.Node, platform string) bool {
	return strings.Contains(strings.ToLower(metaName(doc, "generator")), platform)
}
