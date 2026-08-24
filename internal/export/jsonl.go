package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Xara-AI/sitedex/internal/index"
)

// JSONLRecord is one line of a JSONL export: one indexed chunk, with its
// page's metadata denormalized alongside it (the granularity RAG/embedding
// pipelines generally want).
type JSONLRecord struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Lang        string `json:"lang,omitempty"`
	HeadingPath string `json:"heading_path,omitempty"`
	Ordinal     int    `json:"ordinal"`
	Text        string `json:"text"`
}

// ExportJSONL writes every indexed chunk for site as one JSON object per
// line into <outDir>/<site>.jsonl, and returns the number of records
// written.
func ExportJSONL(dataDir, site, outDir string) (int, error) {
	dbPath := index.Path(dataDir, site)
	if _, err := os.Stat(dbPath); err != nil {
		return 0, fmt.Errorf("no index found for %q (looked in %s) — run `sitedex crawl` first", site, dbPath)
	}
	idx, err := index.Open(dataDir, site)
	if err != nil {
		return 0, fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	chunks, err := idx.AllChunks()
	if err != nil {
		return 0, fmt.Errorf("read chunks: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, fmt.Errorf("create out dir: %w", err)
	}
	outPath := filepath.Join(outDir, site+".jsonl")
	f, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", outPath, err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, c := range chunks {
		rec := JSONLRecord{
			URL:         c.PageURL,
			Title:       c.Title,
			Description: c.Description,
			Lang:        c.Lang,
			HeadingPath: c.HeadingPath,
			Ordinal:     c.Ordinal,
			Text:        c.Text,
		}
		if err := enc.Encode(rec); err != nil {
			return 0, fmt.Errorf("write record: %w", err)
		}
	}
	return len(chunks), nil
}
