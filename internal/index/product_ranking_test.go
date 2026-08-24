package index

import (
	"testing"
	"time"
)

func TestSearch_ProductMatchesByNameAndDescription(t *testing.T) {
	db := openTestDB(t)
	seedProduct(t, db, "https://shop.example.com/shoes", ProductRecord{
		Name: "Blue Nike Running Shoes", Description: "Lightweight and breathable.",
		Price: 129.99, HasPrice: true, Currency: "USD", Availability: "in_stock", ExtractionMethod: "json-ld",
	})

	results, err := db.Search("blue nike shoes", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	r := results[0]
	if r.Type != "product" {
		t.Errorf("Type = %q, want product", r.Type)
	}
	if r.Title != "Blue Nike Running Shoes" {
		t.Errorf("Title = %q", r.Title)
	}
	if !r.HasPrice || r.Price != 129.99 || r.Currency != "USD" {
		t.Errorf("Price=%v HasPrice=%v Currency=%q", r.Price, r.HasPrice, r.Currency)
	}
	if r.Availability != "in_stock" {
		t.Errorf("Availability = %q", r.Availability)
	}
}

func TestSearch_ProductResultPreferredOverPageChunkForSamePage(t *testing.T) {
	db := openTestDB(t)
	url := "https://shop.example.com/shoes"
	err := db.IndexPage(PageRecord{URL: url, Title: "Blue Nike Shoes", CrawledAt: time.Now()}, []ChunkRecord{
		{Ordinal: 0, HeadingPath: "Blue Nike Shoes", Text: "Great blue nike shoes for running long distances."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(url, &ProductRecord{Name: "Blue Nike Shoes", Description: "Running shoes.", ExtractionMethod: "json-ld"}); err != nil {
		t.Fatal(err)
	}

	results, err := db.Search("blue nike shoes", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly 1 (deduped to the product representation)", results)
	}
	if results[0].Type != "product" {
		t.Errorf("Type = %q, want product to win over the page-chunk result for the same URL", results[0].Type)
	}
}

func TestSearch_ExtractionMethodBoostsRanking(t *testing.T) {
	db := openTestDB(t)
	seedProduct(t, db, "https://shop.example.com/a", ProductRecord{
		Name: "Wireless Mouse", Description: "Ergonomic wireless mouse.", ExtractionMethod: "json-ld",
	})
	seedProduct(t, db, "https://shop.example.com/b", ProductRecord{
		Name: "Wireless Mouse", Description: "Ergonomic wireless mouse.", ExtractionMethod: "heuristic",
	})

	results, err := db.Search("wireless mouse", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2", results)
	}
	if results[0].URL != "https://shop.example.com/a" || results[0].ExtractionMethod != "json-ld" {
		t.Errorf("top result = %+v, want the json-ld-extracted product to rank first", results[0])
	}
}

func TestSearch_ProductWithoutPriceOmitsIt(t *testing.T) {
	db := openTestDB(t)
	seedProduct(t, db, "https://shop.example.com/mystery", ProductRecord{
		Name: "Mystery Box", Description: "Contents unknown.", HasPrice: false, ExtractionMethod: "heuristic",
	})
	results, err := db.Search("mystery box", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].HasPrice {
		t.Errorf("HasPrice = true, want false for a product with no known price")
	}
}

func seedProduct(t *testing.T, db *DB, url string, p ProductRecord) {
	t.Helper()
	if err := db.IndexPage(PageRecord{URL: url, Title: p.Name, CrawledAt: time.Now()}, nil); err != nil {
		t.Fatalf("IndexPage(%s): %v", url, err)
	}
	if err := db.IndexProduct(url, &p); err != nil {
		t.Fatalf("IndexProduct(%s): %v", url, err)
	}
}
