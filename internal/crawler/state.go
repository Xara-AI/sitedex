package crawler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PageState is what the crawler remembers about one previously-crawled
// page, enabling conditional revalidation and hash-based skip-on-unchanged
// on the next crawl.
//
// This is a JSON-file stopgap for M2, which has no index yet. Once M3
// lands the SQLite `pages` table (see CLAUDE.md, "Index"), that table
// becomes the source of truth for this data and this file format can
// either be dropped or kept as a lighter-weight cache in front of it.
type PageState struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	Hash         string    `json:"hash,omitempty"`
	CrawledAt    time.Time `json:"crawled_at"`
}

// StateStore persists per-page crawl state for one site as a single JSON
// file, keyed by normalized page URL.
type StateStore struct {
	path string

	mu    sync.Mutex
	pages map[string]PageState
}

// OpenStateStore loads (or initializes) the crawl state file for a site at
// <dataDir>/<site>/crawl-state.json. A missing file is not an error — it
// just means this is the first crawl.
func OpenStateStore(dataDir, site string) (*StateStore, error) {
	path := filepath.Join(dataDir, site, "crawl-state.json")
	s := &StateStore{path: path, pages: make(map[string]PageState)}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read crawl state %s: %w", path, err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.pages); err != nil {
		return nil, fmt.Errorf("parse crawl state %s: %w", path, err)
	}
	return s, nil
}

// Get returns the stored state for a page URL, if any.
func (s *StateStore) Get(pageURL string) (PageState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.pages[pageURL]
	return st, ok
}

// Set records (or replaces) the state for a page URL. It does not write to
// disk — call Save when the crawl finishes (or periodically) to persist.
func (s *StateStore) Set(pageURL string, st PageState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pages[pageURL] = st
}

// Len returns the number of pages tracked.
func (s *StateStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pages)
}

// Save writes the current state to disk atomically (write to a temp file,
// then rename), so a crash or interrupt mid-write can never corrupt the
// previous, still-valid state file.
func (s *StateStore) Save() error {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.pages, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal crawl state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write crawl state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("finalize crawl state: %w", err)
	}
	return nil
}
