package index

import (
	"testing"
	"time"
)

func TestListItems_PageOnly(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://example.com/guide", Title: "Guide", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}

	items, next, err := db.ListItems(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Type != "page" || items[0].URL != page.URL {
		t.Fatalf("items = %+v, want 1 page item", items)
	}
	if next != items[0].Seq || next == 0 {
		t.Errorf("next = %d, want == items[0].Seq (%d) and non-zero", next, items[0].Seq)
	}
}

func TestListItems_ProductWinsOverPage(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", Title: "Shoes Page", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{
		Name: "Blue Shoes", Price: 50, HasPrice: true, Currency: "USD",
		Availability: "in_stock", ExtractionMethod: "json-ld",
	}); err != nil {
		t.Fatal(err)
	}

	items, _, err := db.ListItems(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want 1 (one page, one product on same URL)", items)
	}
	it := items[0]
	if it.Type != "product" || it.Title != "Blue Shoes" || !it.HasPrice || it.Price != 50 {
		t.Errorf("item = %+v, want product representation", it)
	}
}

func TestListItems_SinceSeqOnlyReturnsChanged(t *testing.T) {
	db := openTestDB(t)
	if err := db.IndexPage(PageRecord{URL: "https://example.com/a", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	items, seq1, err := db.ListItems(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want 1", items)
	}

	// Polling again from seq1 with nothing new should return zero items and
	// echo back seq1 as next (checked at the HTTP layer, not here — ListItems
	// itself just returns 0 when nothing matched).
	items2, next2, err := db.ListItems(seq1, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 0 || next2 != 0 {
		t.Fatalf("items2 = %+v, next2 = %d, want none", items2, next2)
	}

	// A second page is a new change and should show up.
	if err := db.IndexPage(PageRecord{URL: "https://example.com/b", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	items3, seq3, err := db.ListItems(seq1, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items3) != 1 || items3[0].URL != "https://example.com/b" {
		t.Fatalf("items3 = %+v, want just the new page", items3)
	}
	if seq3 <= seq1 {
		t.Errorf("seq3 = %d, want > seq1 (%d)", seq3, seq1)
	}
}

func TestListItems_FreshVerifyBumpsSeqWithoutRecrawl(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{Name: "Shoes", Price: 10, HasPrice: true}); err != nil {
		t.Fatal(err)
	}
	_, seq1, err := db.ListItems(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a fresh-verify: product data changes, page row untouched.
	verifiedAt := time.Now()
	if err := db.IndexProduct(page.URL, &ProductRecord{
		Name: "Shoes", Price: 12, HasPrice: true, VerifiedAt: verifiedAt,
	}); err != nil {
		t.Fatal(err)
	}

	items, seq2, err := db.ListItems(seq1, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want the fresh-verified item to show up", items)
	}
	if items[0].VerifiedAt == "" {
		t.Error("VerifiedAt is empty, want the fresh-verify timestamp")
	}
	if seq2 <= seq1 {
		t.Errorf("seq2 = %d, want > seq1 (%d)", seq2, seq1)
	}
}

func TestListItems_TypeFilter(t *testing.T) {
	db := openTestDB(t)
	if err := db.IndexPage(PageRecord{URL: "https://example.com/page", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexPage(PageRecord{URL: "https://example.com/product", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct("https://example.com/product", &ProductRecord{Name: "Widget"}); err != nil {
		t.Fatal(err)
	}

	pages, _, err := db.ListItems(0, "page", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Type != "page" {
		t.Errorf("pages = %+v, want 1 page-type item", pages)
	}

	products, _, err := db.ListItems(0, "product", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].Type != "product" {
		t.Errorf("products = %+v, want 1 product-type item", products)
	}
}
