// Package product implements the product extraction priority chain: JSON-LD
// -> microdata/RDFa -> OpenGraph product tags -> per-platform CSS
// heuristics, stopping at first success. An optional LLM extractor
// (disabled by default) is a pluggable last resort, added in M7 — it does
// not live here.
//
// See CLAUDE.md, "Product extraction".
package product

import (
	"bytes"
	"encoding/json"
	"net/url"

	"golang.org/x/net/html"
)

// Availability is the normalized stock status. Every extraction method
// maps its platform-specific vocabulary onto these three values.
type Availability string

const (
	InStock    Availability = "in_stock"
	OutOfStock Availability = "out_of_stock"
	Unknown    Availability = "unknown"
)

// Method identifies which tier of the extraction chain produced a Product.
// Search ranking boosts higher-confidence methods over lower ones (see
// CLAUDE.md: "JSON-LD > heuristics > llm").
type Method string

const (
	MethodJSONLD    Method = "json-ld"
	MethodMicrodata Method = "microdata"
	MethodOpenGraph Method = "opengraph"
	MethodHeuristic Method = "heuristic"
	MethodLLM       Method = "llm" // reserved for M7; nothing produces this yet
)

// Product is the normalized result of extracting one product from a page.
type Product struct {
	URL              string
	Name             string
	Description      string
	Price            float64
	HasPrice         bool // distinguishes "no price found" from "price is 0"
	Currency         string
	Availability     Availability
	Image            string
	ExtractionMethod Method
	RawJSON          string `json:"-"` // populated by Extract as a debugging artifact; excluded from its own fallback marshaling
}

// Detector is a per-platform CSS-heuristic product detector — the
// designed community-contribution surface (see CONTRIBUTING.md). Each
// detector recognizes one platform's default markup and is tried only
// after JSON-LD, microdata, and OpenGraph have all failed to identify a
// product.
type Detector interface {
	// Name identifies the platform this detector targets, e.g. "woocommerce".
	Name() string
	// Detect attempts to extract a product from doc. ok is false if this
	// platform's markup wasn't recognized on the page.
	Detect(doc *html.Node, pageURL *url.URL) (p *Product, ok bool)
}

// Detectors is the registry of CSS-heuristic detectors, tried in order.
// Add a platform by appending its Detector here.
var Detectors = []Detector{
	wooCommerceDetector{},
	shopifyDetector{},
	prestaShopDetector{},
	openCartDetector{},
}

// Extract runs the product extraction priority chain (JSON-LD -> microdata
// -> OpenGraph -> CSS heuristics) over raw HTML, stopping at the first
// tier that identifies a product. ok is false if no tier matched — most
// pages (articles, category listings, home pages) are not products, and
// that is a normal, expected outcome, not an error.
func Extract(raw []byte, pageURL *url.URL) (*Product, bool) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}

	if p, ok := extractJSONLD(doc, pageURL); ok {
		return finish(p), true
	}
	if p, ok := extractMicrodata(doc, pageURL); ok {
		return finish(p), true
	}
	if p, ok := extractOpenGraph(doc, pageURL); ok {
		return finish(p), true
	}
	for _, d := range Detectors {
		if p, ok := d.Detect(doc, pageURL); ok {
			return finish(p), true
		}
	}
	return nil, false
}

// finish fills in RawJSON with a debugging snapshot of the extracted
// fields when the extraction method didn't already provide one (only
// JSON-LD does, from the original <script> block it parsed).
func finish(p *Product) *Product {
	if p.RawJSON == "" {
		if data, err := json.Marshal(p); err == nil {
			p.RawJSON = string(data)
		}
	}
	return p
}
