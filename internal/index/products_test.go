package index

import (
	"testing"
	"time"
)

func TestIndexProduct_UpsertAndSearch(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", Title: "Blue Shoes", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	err := db.IndexProduct(page.URL, &ProductRecord{
		Name: "Blue Running Shoes", Description: "Lightweight and breathable.",
		Price: 129.99, HasPrice: true, Currency: "USD", Availability: "in_stock",
		Image: "https://shop.example.com/img.jpg", ExtractionMethod: "json-ld", RawJSON: `{"name":"Blue Running Shoes"}`,
	})
	if err != nil {
		t.Fatalf("IndexProduct: %v", err)
	}

	stats, err := db.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.ProductCount != 1 {
		t.Errorf("ProductCount = %d, want 1", stats.ProductCount)
	}

	var name, currency string
	var price float64
	err = db.sql.QueryRow(`SELECT name, currency, price FROM products WHERE page_url = ?`, page.URL).Scan(&name, &currency, &price)
	if err != nil {
		t.Fatalf("query product: %v", err)
	}
	if name != "Blue Running Shoes" || currency != "USD" || price != 129.99 {
		t.Errorf("got name=%q currency=%q price=%v", name, currency, price)
	}
}

func TestIndexProduct_NilRemovesExistingProduct(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", Title: "Blue Shoes", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{Name: "Blue Shoes", HasPrice: false}); err != nil {
		t.Fatal(err)
	}

	stats, _ := db.Stats()
	if stats.ProductCount != 1 {
		t.Fatalf("ProductCount = %d, want 1 before removal", stats.ProductCount)
	}

	if err := db.IndexProduct(page.URL, nil); err != nil {
		t.Fatalf("IndexProduct(nil): %v", err)
	}
	stats, _ = db.Stats()
	if stats.ProductCount != 0 {
		t.Errorf("ProductCount = %d, want 0 after removal", stats.ProductCount)
	}
}

func TestIndexProduct_UpsertReplaces(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{Name: "V1 Name", Price: 10, HasPrice: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{Name: "V2 Name", Price: 20, HasPrice: true}); err != nil {
		t.Fatal(err)
	}

	stats, _ := db.Stats()
	if stats.ProductCount != 1 {
		t.Fatalf("ProductCount = %d, want 1 (upsert, not duplicate)", stats.ProductCount)
	}

	var name string
	var price float64
	if err := db.sql.QueryRow(`SELECT name, price FROM products WHERE page_url = ?`, page.URL).Scan(&name, &price); err != nil {
		t.Fatal(err)
	}
	if name != "V2 Name" || price != 20 {
		t.Errorf("got name=%q price=%v, want V2 Name/20", name, price)
	}
}

func TestIndexProduct_NoPriceStoresNull(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://shop.example.com/shoes", CrawledAt: time.Now()}
	if err := db.IndexPage(page, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexProduct(page.URL, &ProductRecord{Name: "No Price Item", HasPrice: false}); err != nil {
		t.Fatal(err)
	}

	var price interface{}
	if err := db.sql.QueryRow(`SELECT price FROM products WHERE page_url = ?`, page.URL).Scan(&price); err != nil {
		t.Fatal(err)
	}
	if price != nil {
		t.Errorf("price = %v, want NULL", price)
	}
}
