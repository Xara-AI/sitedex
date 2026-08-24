package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyMarkdownKB(t *testing.T) {
	dataDir := t.TempDir()
	kbDir := filepath.Join(dataDir, "example.com", "kb")
	if err := os.MkdirAll(filepath.Join(kbDir, "products"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kbDir, "index.md"), []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kbDir, "products", "shoes.md"), []byte("shoes"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	n, err := CopyMarkdownKB(dataDir, "example.com", outDir)
	if err != nil {
		t.Fatalf("CopyMarkdownKB: %v", err)
	}
	if n != 2 {
		t.Errorf("copied %d files, want 2", n)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "products", "shoes.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "shoes" {
		t.Errorf("content = %q, want shoes", data)
	}
}

func TestCopyMarkdownKB_MissingSiteErrors(t *testing.T) {
	_, err := CopyMarkdownKB(t.TempDir(), "nonexistent.com", t.TempDir())
	if err == nil {
		t.Fatal("expected error for a site with no crawled kb/")
	}
}
