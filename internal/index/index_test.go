package index

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir(), "example.com")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen_CreatesFileAtSitePath(t *testing.T) {
	dataDir := t.TempDir()
	db, err := Open(dataDir, "example.com")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	want := filepath.Join(dataDir, "example.com", "index.db")
	if Path(dataDir, "example.com") != want {
		t.Errorf("Path = %q, want %q", Path(dataDir, "example.com"), want)
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	db1, err := Open(dataDir, "example.com")
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := db1.IndexPage(PageRecord{URL: "https://example.com/", Title: "Home", CrawledAt: time.Now()}, nil); err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(dataDir, "example.com")
	if err != nil {
		t.Fatalf("second Open (re-migrate existing db): %v", err)
	}
	defer func() { _ = db2.Close() }()

	stats, err := db2.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PageCount != 1 {
		t.Errorf("PageCount = %d, want 1 (data should survive reopen)", stats.PageCount)
	}
}

func TestIndexPage_UpsertReplacesChunks(t *testing.T) {
	db := openTestDB(t)

	page := PageRecord{URL: "https://example.com/shoes", Title: "Shoes", CrawledAt: time.Now()}
	err := db.IndexPage(page, []ChunkRecord{
		{Ordinal: 0, HeadingPath: "Shoes", Text: "Blue running shoes"},
		{Ordinal: 1, HeadingPath: "Shoes > Sizes", Text: "Available in sizes 8 to 12"},
	})
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}

	stats, _ := db.Stats()
	if stats.PageCount != 1 || stats.ChunkCount != 2 {
		t.Fatalf("stats after first index = %+v, want 1 page/2 chunks", stats)
	}

	// Re-index with fewer, different chunks: old ones must not linger.
	err = db.IndexPage(page, []ChunkRecord{
		{Ordinal: 0, HeadingPath: "Shoes", Text: "Red walking shoes"},
	})
	if err != nil {
		t.Fatalf("re-IndexPage: %v", err)
	}
	stats, _ = db.Stats()
	if stats.PageCount != 1 || stats.ChunkCount != 1 {
		t.Fatalf("stats after re-index = %+v, want 1 page/1 chunk", stats)
	}

	results, err := db.Search("blue", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected stale chunk text ('blue') to no longer match after re-index, got %+v", results)
	}
}

func TestDeletePage_RemovesChunksAndFTSRows(t *testing.T) {
	db := openTestDB(t)
	page := PageRecord{URL: "https://example.com/gone", Title: "Gone Page", CrawledAt: time.Now()}
	if err := db.IndexPage(page, []ChunkRecord{{Ordinal: 0, Text: "vanishing content"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeletePage(page.URL); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	stats, _ := db.Stats()
	if stats.PageCount != 0 || stats.ChunkCount != 0 {
		t.Errorf("stats after delete = %+v, want all zero", stats)
	}
	results, err := db.Search("vanishing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results after DeletePage, got %+v", results)
	}
}

func TestSearch_EmptyQueryReturnsEmptyNotError(t *testing.T) {
	db := openTestDB(t)
	results, err := db.Search("", 10)
	if err != nil {
		t.Fatalf("Search(\"\"): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want empty", results)
	}

	results, err = db.Search("???", 10)
	if err != nil {
		t.Fatalf("Search(\"???\"): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want empty for all-punctuation query", results)
	}
}

func TestSearch_NoMatchesReturnsEmptyNotError(t *testing.T) {
	db := openTestDB(t)
	if err := db.IndexPage(PageRecord{URL: "https://example.com/", Title: "Home", CrawledAt: time.Now()},
		[]ChunkRecord{{Ordinal: 0, Text: "nothing relevant here"}}); err != nil {
		t.Fatal(err)
	}
	results, err := db.Search("nonexistenttermxyz", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want empty", results)
	}
}

func TestSearch_MultiWordRequiresAllTerms(t *testing.T) {
	db := openTestDB(t)
	seedPage(t, db, "https://example.com/a", "Blue Shoes", "Nice blue shoes for running.")
	seedPage(t, db, "https://example.com/b", "Blue Shirt", "A blue shirt, nothing about footwear.")

	results, err := db.Search("blue shoes", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://example.com/a" {
		t.Errorf("results = %+v, want only /a (both terms must match)", results)
	}
}

// seedPage indexes a single-chunk page for test convenience.
func seedPage(t *testing.T, db *DB, url, title, text string) {
	t.Helper()
	err := db.IndexPage(PageRecord{URL: url, Title: title, CrawledAt: time.Now()}, []ChunkRecord{
		{Ordinal: 0, HeadingPath: title, Text: text},
	})
	if err != nil {
		t.Fatalf("seedPage(%s): %v", url, err)
	}
}
