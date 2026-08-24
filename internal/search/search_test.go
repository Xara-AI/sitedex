package search

import (
	"strings"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/index"
)

func TestSearcher_UncrawledSiteErrors(t *testing.T) {
	_, err := New(t.TempDir()).Search("never-crawled.example", "anything", 10)
	if err == nil {
		t.Fatal("expected an error for a site with no index")
	}
	if !strings.Contains(err.Error(), "no index found") {
		t.Errorf("err = %v, want a no-index-found message", err)
	}
}

func TestSearcher_DelegatesToIndex(t *testing.T) {
	dataDir := t.TempDir()
	idx, err := index.Open(dataDir, "example.com")
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	err = idx.IndexPage(index.PageRecord{URL: "https://example.com/shoes", Title: "Blue Shoes", CrawledAt: time.Now()},
		[]index.ChunkRecord{{Ordinal: 0, HeadingPath: "Shoes", Text: "Nice blue running shoes"}})
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	results, err := New(dataDir).Search("example.com", "blue shoes", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Blue Shoes" {
		t.Errorf("results = %+v, want 1 result titled Blue Shoes", results)
	}
}
