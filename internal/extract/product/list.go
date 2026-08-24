package product

import (
	"bytes"
	"encoding/json"
	"net/url"

	"golang.org/x/net/html"
)

// maxListItems caps how many products ExtractList returns from a single
// page — a defensive bound, not a claim that a page can't legitimately
// list more.
const maxListItems = 20

// listingCardClasses are common container classes for one product "card"
// in a search-results/category grid, across several popular platforms.
// This is intentionally a best-effort, generic fallback — the JSON-LD
// path in ExtractList covers the more reliable case, and per-platform
// single-product Detectors (heuristics_*.go) aren't reused here since a
// listing card carries far less markup than a full product page.
var listingCardClasses = []string{
	"product", // WooCommerce (ul.products > li.product), generic
	"product-item",
	"product-card",
	"grid__item",        // Shopify
	"product-miniature", // PrestaShop
	"product-thumb",     // OpenCart
	"product-layout",    // OpenCart (older themes)
}

var listingTitleClasses = []string{
	"woocommerce-loop-product__title",
	"product-item-meta__title",
	"product-title",
	"card__heading",
}

var listingPriceClasses = []string{
	"woocommerce-Price-amount",
	"price-item",
	"price",
	"product-price",
}

// ExtractList extracts every product on a page that lists many (a
// search-results or category page), trying JSON-LD first — a single
// script block commonly contains an ItemList of Products, or simply
// multiple Product nodes — then falling back to a generic repeated-card
// CSS heuristic. It returns nil if neither approach finds anything;
// that's a normal outcome (most listing pages a crawler doesn't
// specifically know about won't match the heuristic), not an error.
func ExtractList(raw []byte, pageURL *url.URL) []*Product {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil
	}

	if products := extractJSONLDList(doc, pageURL); len(products) > 0 {
		return products
	}
	return extractCardList(doc, pageURL)
}

func extractJSONLDList(doc *html.Node, pageURL *url.URL) []*Product {
	var out []*Product
	for _, raw := range jsonLDScripts(doc) {
		var v interface{}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			continue
		}
		for _, c := range findProductCandidates(v) {
			p := productFromJSONLD(c, pageURL)
			if p.Name == "" {
				continue
			}
			p.RawJSON = raw
			out = append(out, p)
			if len(out) >= maxListItems {
				return out
			}
		}
	}
	return out
}

func extractCardList(doc *html.Node, pageURL *url.URL) []*Product {
	seen := make(map[*html.Node]bool)
	var cards []*html.Node
	for _, class := range listingCardClasses {
		for _, n := range findAllByClass(doc, class) {
			if seen[n] {
				continue
			}
			seen[n] = true
			cards = append(cards, n)
		}
	}

	var out []*Product
	for _, card := range cards {
		if p := productFromCard(card, pageURL); p != nil {
			out = append(out, p)
		}
		if len(out) >= maxListItems {
			break
		}
	}
	return out
}

// productFromCard extracts a minimal product from one listing card: it
// needs at least a link (to the product's own page) and a name to be
// worth returning. Price, image, and availability are filled in on a
// best-effort basis.
func productFromCard(card *html.Node, pageURL *url.URL) *Product {
	link := findByTag(card, "a")
	if link == nil {
		return nil
	}
	href := attrVal(link, "href")
	if href == "" {
		return nil
	}

	name := textOfAnyClass(card, listingTitleClasses...)
	if name == "" {
		if h := findByAnyTag(card, "h2", "h3"); h != nil {
			name = textContent(h)
		}
	}
	if name == "" {
		name = textContent(link)
	}
	if name == "" {
		return nil
	}

	priceText := textOfAnyClass(card, listingPriceClasses...)
	price, hasPrice := priceField(priceText)

	img := ""
	if imgNode := findByTag(card, "img"); imgNode != nil {
		img = firstNonEmpty(attrVal(imgNode, "src"), attrVal(imgNode, "data-src"))
	}

	return &Product{
		URL: resolveURL(pageURL, href), Name: normalizeSpace(name),
		Price: price, HasPrice: hasPrice, Currency: currencyFromSymbol(priceText),
		Availability: Unknown, Image: resolveURL(pageURL, img),
		ExtractionMethod: MethodHeuristic,
	}
}

// findAllByClass returns every element under n with the given class,
// without descending into an already-matched element's subtree (a
// card's internal markup is very unlikely to contain another top-level
// matching card, and not recursing avoids double-counting nested
// structure).
func findAllByClass(n *html.Node, class string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, class) {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func findByAnyTag(n *html.Node, tags ...string) *html.Node {
	for _, tag := range tags {
		if found := findByTag(n, tag); found != nil {
			return found
		}
	}
	return nil
}
