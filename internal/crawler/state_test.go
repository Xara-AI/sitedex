package crawler

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateStore_MissingFileStartsEmpty(t *testing.T) {
	s, err := OpenStateStore(t.TempDir(), "example.com")
	if err != nil {
		t.Fatalf("OpenStateStore: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0", s.Len())
	}
	if _, ok := s.Get("https://example.com/x"); ok {
		t.Error("expected no state for unknown page")
	}
}

func TestStateStore_SetSaveReload(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStateStore(dir, "example.com")
	if err != nil {
		t.Fatalf("OpenStateStore: %v", err)
	}

	want := PageState{
		ETag:         `"abc"`,
		LastModified: "Mon, 02 Jan 2006 15:04:05 GMT",
		Hash:         "deadbeef",
		CrawledAt:    time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}
	s.Set("https://example.com/products/1", want)
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := OpenStateStore(dir, "example.com")
	if err != nil {
		t.Fatalf("reload OpenStateStore: %v", err)
	}
	got, ok := reloaded.Get("https://example.com/products/1")
	if !ok {
		t.Fatal("expected state to be present after reload")
	}
	if got.ETag != want.ETag || got.Hash != want.Hash || got.LastModified != want.LastModified {
		t.Errorf("got = %+v, want %+v", got, want)
	}
	if !got.CrawledAt.Equal(want.CrawledAt) {
		t.Errorf("CrawledAt = %v, want %v", got.CrawledAt, want.CrawledAt)
	}
}

func TestStateStore_SavesUnderSiteSubdir(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStateStore(dir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.Set("https://example.com/x", PageState{Hash: "h"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "example.com", "crawl-state.json")
	if s.path != want {
		t.Errorf("path = %q, want %q", s.path, want)
	}
}

func TestStateStore_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStateStore(dir, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.Set("https://example.com/x", PageState{Hash: "h"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	// Corrupt the file, then verify reopening surfaces an error instead of
	// silently discarding history.
	writeFileForTest(t, s.path, "not json")
	if _, err := OpenStateStore(dir, "example.com"); err == nil {
		t.Error("expected error opening corrupt state file")
	}
}
