package index

import "testing"

func TestSearchFiltered_TypeFilter(t *testing.T) {
	db := openTestDB(t)
	seedPage(t, db, "https://example.com/blog/shoes-guide", "Shoe Buying Guide", "A guide about running shoes and how to pick the right pair.")
	seedProduct(t, db, "https://example.com/products/shoes", ProductRecord{
		Name: "Running Shoes", Description: "Great running shoes.", ExtractionMethod: "json-ld",
	})

	t.Run("any", func(t *testing.T) {
		results, err := db.SearchFiltered("running shoes", 10, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("results = %+v, want 2", results)
		}
	})

	t.Run("page only", func(t *testing.T) {
		results, err := db.SearchFiltered("running shoes", 10, "page")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Type != "page" {
			t.Fatalf("results = %+v, want 1 page result", results)
		}
	})

	t.Run("product only", func(t *testing.T) {
		results, err := db.SearchFiltered("running shoes", 10, "product")
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Type != "product" {
			t.Fatalf("results = %+v, want 1 product result", results)
		}
	})
}

func TestSearchFiltered_ProductTypeStillCapsAtLimit(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 5; i++ {
		seedProduct(t, db, "https://example.com/p"+string(rune('a'+i)), ProductRecord{
			Name: "Blue Widget", Description: "A widget.", ExtractionMethod: "heuristic",
		})
	}
	results, err := db.SearchFiltered("blue widget", 3, "product")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("results len = %d, want 3 (limit respected even when filtered)", len(results))
	}
}
