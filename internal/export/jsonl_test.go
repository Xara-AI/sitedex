package export

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Xara-AI/sitedex/internal/index"
)

func TestExportJSONL(t *testing.T) {
	dataDir := t.TempDir()
	idx, err := index.Open(dataDir, "example.com")
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	err = idx.IndexPage(index.PageRecord{
		URL: "https://example.com/shoes", Title: "Shoes", Description: "Nice shoes", Lang: "en", CrawledAt: time.Now(),
	}, []index.ChunkRecord{
		{Ordinal: 0, HeadingPath: "Shoes", Text: "Blue running shoes"},
		{Ordinal: 1, HeadingPath: "Shoes > Sizes", Text: "Sizes 8 to 12"},
	})
	if err != nil {
		t.Fatalf("IndexPage: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	n, err := ExportJSONL(dataDir, "example.com", outDir)
	if err != nil {
		t.Fatalf("ExportJSONL: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}

	f, err := os.Open(filepath.Join(outDir, "example.com.jsonl"))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer func() { _ = f.Close() }()

	var records []JSONLRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec JSONLRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal line %q: %v", sc.Text(), err)
		}
		records = append(records, rec)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d JSONL records, want 2", len(records))
	}
	if records[0].URL != "https://example.com/shoes" || records[0].Title != "Shoes" || records[0].Text != "Blue running shoes" {
		t.Errorf("records[0] = %+v", records[0])
	}
	if records[1].Ordinal != 1 || records[1].HeadingPath != "Shoes > Sizes" {
		t.Errorf("records[1] = %+v", records[1])
	}
}

func TestExportJSONL_MissingSiteErrors(t *testing.T) {
	_, err := ExportJSONL(t.TempDir(), "nonexistent.com", t.TempDir())
	if err == nil {
		t.Fatal("expected error for a site with no index")
	}
}
