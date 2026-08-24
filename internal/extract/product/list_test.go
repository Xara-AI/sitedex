package product

import "testing"

func TestExtractList_JSONLDItemList(t *testing.T) {
	raw := loadFixture(t, "jsonld-itemlist.html")
	products := ExtractList(raw, mustURL(t, "https://shop.example.com/search?q=shoes"))

	if len(products) != 3 {
		t.Fatalf("got %d products, want 3: %+v", len(products), products)
	}

	if products[0].Name != "Blue Running Shoes" {
		t.Errorf("products[0].Name = %q", products[0].Name)
	}
	if products[0].URL != "https://shop.example.com/products/blue-running-shoes" {
		t.Errorf("products[0].URL = %q, want the item's own resolved URL, not the search page's", products[0].URL)
	}
	if !products[0].HasPrice || products[0].Price != 129.99 {
		t.Errorf("products[0] price = %v/%v, want 129.99", products[0].Price, products[0].HasPrice)
	}

	if products[1].Name != "Red Running Shoes" || products[1].Availability != OutOfStock {
		t.Errorf("products[1] = %+v, want Red Running Shoes/out_of_stock", products[1])
	}

	if products[2].URL != "https://shop.example.com/products/trail-hiking-boots" {
		t.Errorf("products[2].URL = %q", products[2].URL)
	}

	for i, p := range products {
		if p.ExtractionMethod != MethodJSONLD {
			t.Errorf("products[%d].ExtractionMethod = %q, want json-ld", i, p.ExtractionMethod)
		}
	}
}

func TestExtractList_WooCommerceCardGrid(t *testing.T) {
	raw := loadFixture(t, "woocommerce-listing.html")
	products := ExtractList(raw, mustURL(t, "https://shop.example.com/?s=shoes"))

	if len(products) != 2 {
		t.Fatalf("got %d products, want 2: %+v", len(products), products)
	}

	p := products[0]
	if p.Name != "Blue Running Shoes" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.URL != "https://shop.example.com/product/blue-shoes/" {
		t.Errorf("URL = %q", p.URL)
	}
	if !p.HasPrice || p.Price != 129.99 {
		t.Errorf("Price = %v/%v, want 129.99", p.Price, p.HasPrice)
	}
	if p.Image != "https://shop.example.com/wp-content/uploads/blue-shoes-thumb.jpg" {
		t.Errorf("Image = %q", p.Image)
	}
	if p.ExtractionMethod != MethodHeuristic {
		t.Errorf("ExtractionMethod = %q, want heuristic", p.ExtractionMethod)
	}

	if products[1].Name != "Red Running Shoes" {
		t.Errorf("products[1].Name = %q", products[1].Name)
	}
}

func TestExtractList_NoMatchesReturnsNil(t *testing.T) {
	raw := loadFixture(t, "not-a-product-article.html")
	products := ExtractList(raw, mustURL(t, "https://example.com/blog/sourcing"))
	if len(products) != 0 {
		t.Errorf("got %d products, want 0: %+v", len(products), products)
	}
}

func TestExtractList_CapsAtMaxListItems(t *testing.T) {
	var sb []byte
	sb = append(sb, []byte(`<html><body><ul class="products">`)...)
	for i := 0; i < maxListItems+10; i++ {
		sb = append(sb, []byte(`<li class="product"><a href="/p`+string(rune('a'+i%26))+`">Item Name Padding Text</a></li>`)...)
	}
	sb = append(sb, []byte(`</ul></body></html>`)...)

	products := ExtractList(sb, mustURL(t, "https://example.com/"))
	if len(products) != maxListItems {
		t.Errorf("got %d products, want capped at %d", len(products), maxListItems)
	}
}

func TestExtractList_MalformedHTMLDoesNotPanic(t *testing.T) {
	_ = ExtractList([]byte("<html><body><li class=product<<<"), mustURL(t, "https://example.com/"))
}
