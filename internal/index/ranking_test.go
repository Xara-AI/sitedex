package index

import (
	"testing"
	"time"
)

// fixture is a small, representative corpus (product-search style pages)
// shared by the ranking regression tests below. Assertions check top-N
// ordering, per CLAUDE.md's testing requirements — not exact scores,
// which are a display heuristic, not a contract.
func fixture(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)

	pages := []struct {
		url, title, headingPath, text string
	}{
		{
			"https://shop.example.com/blue-nike-shoes",
			"Blue Nike Running Shoes",
			"Shoes > Running",
			"Lightweight running shoes in a vivid blue colorway, built for long distances.",
		},
		{
			"https://shop.example.com/red-nike-shoes",
			"Red Nike Running Shoes",
			"Shoes > Running",
			"The same great running shoe, now in red. Blue laces included as an accent.",
		},
		{
			"https://shop.example.com/blue-shirt",
			"Blue Cotton Shirt",
			"Apparel > Shirts",
			"A comfortable blue shirt made from 100% cotton, perfect for everyday wear.",
		},
		{
			"https://shop.example.com/hiking-guide",
			"Hiking Gear Guide",
			"Guides > Hiking",
			"Buying good shoes matters for hiking. Consider fit, grip, and whether the color blue or black hides trail dust better.",
		},
		{
			"https://shop.example.com/pantofi-alergare",
			"Pantofi de alergare albaștri",
			"Pantofi > Alergare",
			"Pantofi ușori pentru alergare pe distanțe lungi, culoare albastru intens.",
		},
	}
	for _, p := range pages {
		err := db.IndexPage(PageRecord{URL: p.url, Title: p.title, CrawledAt: time.Now()}, []ChunkRecord{
			{Ordinal: 0, HeadingPath: p.headingPath, Text: p.text},
		})
		if err != nil {
			t.Fatalf("IndexPage(%s): %v", p.url, err)
		}
	}
	return db
}

func TestRanking_TitleMatchOutranksBodyOnlyMatch(t *testing.T) {
	db := fixture(t)

	results, err := db.Search("blue shoes", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	// "blue-nike-shoes" has both terms in its title; the hiking guide only
	// has "shoes" in the title and "blue" in body text discussing trail
	// dust color, unrelated to footwear color. The title match should win.
	if results[0].URL != "https://shop.example.com/blue-nike-shoes" {
		t.Errorf("top result = %q, want blue-nike-shoes; full order: %+v", results[0].URL, urlsOf(results))
	}
}

func TestRanking_ExactPhraseBeatsSameWordsAnyOrder(t *testing.T) {
	db := openTestDB(t)
	// Both pages contain "running" and "shoes"; only one contains them as
	// the exact adjacent phrase "running shoes".
	seedPage(t, db, "https://example.com/exact", "Gear", "These running shoes are excellent for the trail.")
	seedPage(t, db, "https://example.com/scattered", "Gear", "For running, comfortable shoes matter more than looks.")

	results, err := db.Search("running shoes", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	if results[0].URL != "https://example.com/exact" {
		t.Errorf("top result = %q, want the exact-phrase page; order: %+v", results[0].URL, urlsOf(results))
	}
}

func TestRanking_RomanianDiacriticsMatchUnaccentedQuery(t *testing.T) {
	db := fixture(t)

	// Query has no diacritics; indexed text does ("albaștri", "ușori",
	// "distanțe", "albastru"). remove_diacritics=2 should fold both.
	results, err := db.Search("pantofi alergare", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 || results[0].URL != "https://shop.example.com/pantofi-alergare" {
		t.Errorf("results = %+v, want pantofi-alergare first for unaccented query", urlsOf(results))
	}
}

func TestRanking_AccentedQueryMatchesUnaccentedOrAccentedText(t *testing.T) {
	db := fixture(t)

	// Same query, now WITH diacritics, should still find the page.
	results, err := db.Search("pantofi albaștri", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 || results[0].URL != "https://shop.example.com/pantofi-alergare" {
		t.Errorf("results = %+v, want pantofi-alergare first for accented query", urlsOf(results))
	}
}

func TestRanking_CaseInsensitive(t *testing.T) {
	db := fixture(t)

	lower, err := db.Search("blue nike shoes", 5)
	if err != nil {
		t.Fatal(err)
	}
	upper, err := db.Search("BLUE NIKE SHOES", 5)
	if err != nil {
		t.Fatal(err)
	}
	mixed, err := db.Search("Blue Nike Shoes", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(lower) == 0 || len(upper) == 0 || len(mixed) == 0 {
		t.Fatalf("expected results for all casings: lower=%v upper=%v mixed=%v", lower, upper, mixed)
	}
	if lower[0].URL != upper[0].URL || lower[0].URL != mixed[0].URL {
		t.Errorf("top result differs by case: lower=%s upper=%s mixed=%s", lower[0].URL, upper[0].URL, mixed[0].URL)
	}
}

func TestRanking_LimitIsRespected(t *testing.T) {
	db := fixture(t)
	results, err := db.Search("shoes", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestRanking_ScoresAreDescending(t *testing.T) {
	db := fixture(t)
	results, err := db.Search("running shoes", 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("scores not descending at index %d: %+v", i, urlsOf(results))
		}
	}
}

func urlsOf(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.URL
	}
	return out
}
