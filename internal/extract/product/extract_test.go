package product

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

func TestExtract_JSONLD_Basic(t *testing.T) {
	raw := loadFixture(t, "jsonld-basic.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/products/blue-shoes"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.ExtractionMethod != MethodJSONLD {
		t.Errorf("ExtractionMethod = %q, want json-ld", p.ExtractionMethod)
	}
	if p.Name != "Blue Running Shoes" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Description != "Lightweight running shoes built for long distances." {
		t.Errorf("Description = %q", p.Description)
	}
	if !p.HasPrice || p.Price != 129.99 {
		t.Errorf("Price = %v (HasPrice=%v), want 129.99", p.Price, p.HasPrice)
	}
	if p.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
	if p.Image != "https://shop.example.com/img/blue-shoes.jpg" {
		t.Errorf("Image = %q, want resolved absolute URL", p.Image)
	}
	if p.RawJSON == "" {
		t.Error("expected RawJSON to hold the original JSON-LD block")
	}
	if p.URL != "https://shop.example.com/products/blue-shoes" {
		t.Errorf("URL = %q", p.URL)
	}
}

func TestExtract_JSONLD_ArrayImageAndNumericPrice(t *testing.T) {
	raw := loadFixture(t, "jsonld-array-of-images-and-numeric-price.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/shirt"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.Name != "Red Cotton Shirt" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Image != "https://shop.example.com/img/shirt-1.jpg" {
		t.Errorf("Image = %q, want first array element resolved", p.Image)
	}
	if !p.HasPrice || p.Price != 39.5 {
		t.Errorf("Price = %v (HasPrice=%v), want 39.5 (numeric JSON price)", p.Price, p.HasPrice)
	}
	if p.Availability != OutOfStock {
		t.Errorf("Availability = %q, want out_of_stock", p.Availability)
	}
}

func TestExtract_JSONLD_Graph(t *testing.T) {
	raw := loadFixture(t, "jsonld-graph.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/mug"))
	if !ok {
		t.Fatal("expected a product to be extracted from within @graph")
	}
	if p.Name != "Ceramic Mug" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 14.99 {
		t.Errorf("Price = %v (HasPrice=%v), want 14.99 (European decimal comma \"14,99\")", p.Price, p.HasPrice)
	}
	if p.Currency != "RON" {
		t.Errorf("Currency = %q, want RON", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock (bare \"InStock\" value)", p.Availability)
	}
}

func TestExtract_JSONLD_AggregateOffer(t *testing.T) {
	raw := loadFixture(t, "jsonld-aggregate-offer.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/headphones"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if !p.HasPrice || p.Price != 199.0 {
		t.Errorf("Price = %v (HasPrice=%v), want 199.0 (AggregateOffer.lowPrice)", p.Price, p.HasPrice)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock (LimitedAvailability)", p.Availability)
	}
}

func TestExtract_JSONLD_MalformedBlockIsSkipped(t *testing.T) {
	raw := loadFixture(t, "jsonld-malformed-then-valid.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/wallet"))
	if !ok {
		t.Fatal("expected extraction to skip the malformed block and use the valid one")
	}
	if p.Name != "Leather Wallet" {
		t.Errorf("Name = %q", p.Name)
	}
}

func TestExtract_NotAProduct(t *testing.T) {
	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := Extract(raw, mustURL(t, "https://shop.example.com/blog/sourcing"))
	if ok {
		t.Error("expected no product to be extracted from a plain article page")
	}
}

func TestExtract_Microdata(t *testing.T) {
	raw := loadFixture(t, "microdata-basic.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/yoga-mat"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.ExtractionMethod != MethodMicrodata {
		t.Errorf("ExtractionMethod = %q, want microdata", p.ExtractionMethod)
	}
	if p.Name != "Yoga Mat" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Description != "Non-slip yoga mat, 6mm thick." {
		t.Errorf("Description = %q", p.Description)
	}
	if !p.HasPrice || p.Price != 45.0 {
		t.Errorf("Price = %v (HasPrice=%v), want 45.0", p.Price, p.HasPrice)
	}
	if p.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (from <meta content>)", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock (from <link href>)", p.Availability)
	}
	if p.Image != "https://shop.example.com/img/yoga-mat.jpg" {
		t.Errorf("Image = %q", p.Image)
	}
}

func TestExtract_OpenGraph(t *testing.T) {
	raw := loadFixture(t, "opengraph-basic.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/desk-lamp"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.ExtractionMethod != MethodOpenGraph {
		t.Errorf("ExtractionMethod = %q, want opengraph", p.ExtractionMethod)
	}
	if p.Name != "Desk Lamp" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 34.99 {
		t.Errorf("Price = %v (HasPrice=%v), want 34.99", p.Price, p.HasPrice)
	}
	if p.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
}

func TestExtract_OpenGraph_NonProductTypeDoesNotMatch(t *testing.T) {
	raw := []byte(`<html><head><meta property="og:type" content="article"><meta property="og:title" content="Some Article"></head><body></body></html>`)
	_, ok := Extract(raw, mustURL(t, "https://example.com/blog/post"))
	if ok {
		t.Error("expected og:type=article to not be extracted as a product")
	}
}

func TestExtract_WooCommerce(t *testing.T) {
	raw := loadFixture(t, "woocommerce.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/backpack"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.ExtractionMethod != MethodHeuristic {
		t.Errorf("ExtractionMethod = %q, want heuristic", p.ExtractionMethod)
	}
	if p.Name != "Canvas Backpack" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 79.0 {
		t.Errorf("Price = %v (HasPrice=%v), want 79.0", p.Price, p.HasPrice)
	}
	if p.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (from $ symbol)", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
	if p.Image != "https://shop.example.com/wp-content/uploads/canvas-backpack.jpg" {
		t.Errorf("Image = %q", p.Image)
	}
}

func TestExtract_WooCommerce_RequiresWooCommerceMarker(t *testing.T) {
	// A generic ".product" div with no ".woocommerce" wrapper anywhere on
	// the page should NOT be claimed by the WooCommerce detector.
	raw := []byte(`<html><body><div class="product"><h1 class="product_title">Thing</h1></div></body></html>`)
	p, ok := wooCommerceDetector{}.Detect(mustParseDoc(t, raw), mustURL(t, "https://example.com/x"))
	if ok {
		t.Errorf("expected no match without a .woocommerce marker, got %+v", p)
	}
}

func TestExtract_Shopify(t *testing.T) {
	raw := loadFixture(t, "shopify.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/plant-pot"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.ExtractionMethod != MethodHeuristic {
		t.Errorf("ExtractionMethod = %q, want heuristic", p.ExtractionMethod)
	}
	if p.Name != "Ceramic Plant Pot" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 24.0 {
		t.Errorf("Price = %v (HasPrice=%v), want 24.0", p.Price, p.HasPrice)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
	if p.Image != "https://cdn.shopify.com/s/files/1/0000/0000/products/pot.jpg" {
		t.Errorf("Image = %q, want protocol-relative src resolved against https base", p.Image)
	}
}

func TestExtract_Shopify_SoldOut(t *testing.T) {
	raw := loadFixture(t, "shopify-sold-out.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/scarf"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.Availability != OutOfStock {
		t.Errorf("Availability = %q, want out_of_stock", p.Availability)
	}
}

func TestExtract_PrestaShop(t *testing.T) {
	raw := loadFixture(t, "prestashop.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/table-lamp"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.Name != "Table Lamp" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 129.9 {
		t.Errorf("Price = %v (HasPrice=%v), want 129.9 (\"129,90 lei\")", p.Price, p.HasPrice)
	}
	if p.Currency != "RON" {
		t.Errorf("Currency = %q, want RON", p.Currency)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
}

func TestExtract_OpenCart(t *testing.T) {
	raw := loadFixture(t, "opencart.html")
	p, ok := Extract(raw, mustURL(t, "https://shop.example.com/garden-hose"))
	if !ok {
		t.Fatal("expected a product to be extracted")
	}
	if p.Name != "Garden Hose" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 29.99 {
		t.Errorf("Price = %v (HasPrice=%v), want 29.99", p.Price, p.HasPrice)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
}

func TestExtract_PriorityChain_JSONLDBeatsMicrodata(t *testing.T) {
	// A page with both JSON-LD and microdata present must use JSON-LD
	// (first in the priority chain), even though the microdata says
	// something different.
	raw := []byte(`<html><head><script type="application/ld+json">
{"@type":"Product","name":"From JSON-LD","offers":{"@type":"Offer","price":"10.00","priceCurrency":"USD","availability":"https://schema.org/InStock"}}
</script></head>
<body>
<div itemscope itemtype="https://schema.org/Product">
  <span itemprop="name">From Microdata</span>
</div>
</body></html>`)
	p, ok := Extract(raw, mustURL(t, "https://example.com/x"))
	if !ok {
		t.Fatal("expected extraction to succeed")
	}
	if p.Name != "From JSON-LD" || p.ExtractionMethod != MethodJSONLD {
		t.Errorf("got Name=%q Method=%q, want JSON-LD to win the priority chain", p.Name, p.ExtractionMethod)
	}
}

func TestExtract_MalformedHTMLDoesNotPanic(t *testing.T) {
	_, _ = Extract([]byte("<html><body><div class=woocommerce<<<"), mustURL(t, "https://example.com/"))
}
