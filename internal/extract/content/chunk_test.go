package content

import (
	"strings"
	"testing"
)

func TestChunk_SplitsOnHeadingBoundaries(t *testing.T) {
	page := &Page{Markdown: "# Shoes\n\nIntro to shoes.\n\n## Running\n\nRunning shoes content.\n\n## Casual\n\nCasual shoes content."}
	chunks := ChunkPage(page, 1200, 100)

	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3: %+v", len(chunks), chunks)
	}
	if chunks[0].HeadingPath != "Shoes" || !strings.Contains(chunks[0].Text, "Intro to shoes") {
		t.Errorf("chunks[0] = %+v", chunks[0])
	}
	if chunks[1].HeadingPath != "Shoes > Running" || !strings.Contains(chunks[1].Text, "Running shoes content") {
		t.Errorf("chunks[1] = %+v", chunks[1])
	}
	if chunks[2].HeadingPath != "Shoes > Casual" || !strings.Contains(chunks[2].Text, "Casual shoes content") {
		t.Errorf("chunks[2] = %+v", chunks[2])
	}
}

func TestChunk_OrdinalsAreSequential(t *testing.T) {
	page := &Page{Markdown: "# A\n\ntext a\n\n# B\n\ntext b"}
	chunks := ChunkPage(page, 1200, 100)
	for i, c := range chunks {
		if c.Ordinal != i {
			t.Errorf("chunks[%d].Ordinal = %d, want %d", i, c.Ordinal, i)
		}
	}
}

func TestChunk_LargeSectionSplitsWithOverlap(t *testing.T) {
	// Build a section with no internal whitespace-friendly break points near
	// the target boundary, long enough to require multiple pieces.
	body := strings.Repeat("a", 3000)
	page := &Page{Markdown: "# Big\n\n" + body}
	chunks := ChunkPage(page, 1000, 100)

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks for a 3000-char section at target 1000, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.HeadingPath != "Big" {
			t.Errorf("chunk heading path = %q, want Big", c.HeadingPath)
		}
		if len([]rune(c.Text)) > 1000 {
			t.Errorf("chunk text length %d exceeds target 1000", len([]rune(c.Text)))
		}
	}
}

func TestChunk_PreambleBeforeFirstHeadingHasEmptyPath(t *testing.T) {
	page := &Page{Markdown: "Some intro text with no heading yet.\n\n# First Heading\n\nBody."}
	chunks := ChunkPage(page, 1200, 100)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0].HeadingPath != "" {
		t.Errorf("preamble HeadingPath = %q, want empty", chunks[0].HeadingPath)
	}
	if chunks[1].HeadingPath != "First Heading" {
		t.Errorf("chunks[1].HeadingPath = %q, want First Heading", chunks[1].HeadingPath)
	}
}

func TestChunk_HeadingPathPopsOnLowerLevel(t *testing.T) {
	page := &Page{Markdown: "# A\n\n## B\n\ntext b\n\n# C\n\ntext c"}
	chunks := ChunkPage(page, 1200, 100)
	var paths []string
	for _, c := range chunks {
		paths = append(paths, c.HeadingPath)
	}
	// "A > B" section, then a new top-level "C" should NOT be "A > C".
	found := false
	for _, p := range paths {
		if p == "C" {
			found = true
		}
		if p == "A > C" {
			t.Errorf("heading path incorrectly retained stale parent: %v", paths)
		}
	}
	if !found {
		t.Errorf("expected a chunk with heading path %q, got %v", "C", paths)
	}
}

func TestChunk_EmptyMarkdownYieldsNoChunks(t *testing.T) {
	page := &Page{Markdown: ""}
	if chunks := ChunkPage(page, 1200, 100); len(chunks) != 0 {
		t.Errorf("got %d chunks for empty markdown, want 0", len(chunks))
	}
}

func TestChunk_DefaultsGuardAgainstBadInput(t *testing.T) {
	page := &Page{Markdown: "# H\n\n" + strings.Repeat("word ", 400)}
	// targetChars <= 0 and overlap >= target should not panic or infinite-loop.
	done := make(chan []Chunk, 1)
	go func() { done <- ChunkPage(page, 0, 5000) }()
	select {
	case chunks := <-done:
		if len(chunks) == 0 {
			t.Error("expected at least one chunk with defaulted target size")
		}
	case <-timeoutCh():
		t.Fatal("Chunk did not return, likely an infinite loop with bad input")
	}
}
