package index

import "testing"

// TestSearchSoft_RelaxesInflectedSuffix reproduces the reported production
// incident: a visitor's query carries grammatical agreement ("externalizată",
// feminine, agreeing with "echipă") that the site's own copy doesn't
// ("externalizat"). Exact-token search must still return zero (that's the
// documented, predictable default); SearchSoft must find the page anyway.
func TestSearchSoft_RelaxesInflectedSuffix(t *testing.T) {
	db := openTestDB(t)
	seedPage(t, db, "https://example.com/echipa-marketing",
		"Echipa tehnică de marketing externalizat",
		"Serviciul nostru de echipă de marketing externalizat acoperă strategie, conținut și performanță.",
	)

	const query = "echipă marketing externalizată"

	strict, err := db.SearchFiltered(query, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(strict) != 0 {
		t.Fatalf("SearchFiltered(%q) = %+v, want 0 results (exact-token search must stay strict)", query, strict)
	}

	soft, err := db.SearchSoft(query, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(soft) != 1 || soft[0].URL != "https://example.com/echipa-marketing" {
		t.Fatalf("SearchSoft(%q) = %+v, want the echipa-marketing page", query, soft)
	}
}

// TestSearchSoft_SkipsRelaxationWhenStrictAlreadyHits ensures SearchSoft
// doesn't do extra work (or risk widening results) when the exact query
// already matches — it should return exactly what SearchFiltered would.
func TestSearchSoft_SkipsRelaxationWhenStrictAlreadyHits(t *testing.T) {
	db := openTestDB(t)
	seedPage(t, db, "https://example.com/shoes", "Blue Running Shoes", "Lightweight running shoes in blue.")

	strict, err := db.SearchFiltered("blue running shoes", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	soft, err := db.SearchSoft("blue running shoes", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(soft) != len(strict) || len(soft) != 1 {
		t.Fatalf("SearchSoft = %+v, want same single result as SearchFiltered %+v", soft, strict)
	}
}

// TestSearchSoft_StillEmptyWhenNothingSharesAStem confirms SearchSoft
// doesn't degrade into an unbounded fuzzy search: an unrelated query still
// comes back empty rather than matching everything.
func TestSearchSoft_StillEmptyWhenNothingSharesAStem(t *testing.T) {
	db := openTestDB(t)
	seedPage(t, db, "https://example.com/shoes", "Blue Running Shoes", "Lightweight running shoes in blue.")

	results, err := db.SearchSoft("wireless bluetooth headphones", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchSoft(unrelated query) = %+v, want 0 results", results)
	}
}
