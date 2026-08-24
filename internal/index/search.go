package index

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Result is one page-level search hit: the best-scoring chunk of a page,
// representing that page in the results (a page can have many chunks, but
// only its best match is surfaced, matching the HTTP API's one-row-per-page
// contract described in CLAUDE.md).
type Result struct {
	URL         string
	Title       string
	HeadingPath string
	Snippet     string
	Score       float64 // heuristic, roughly 0..1, higher is better
}

// bm25Weights assigns column weights for chunks_fts (page_url, ord, title,
// heading_path, text, in schema order): title matches rank well above
// heading-path matches, which rank above plain body text. page_url/ord are
// UNINDEXED and never match, so their weight is irrelevant.
const bm25Weights = "0, 0, 5.0, 2.0, 1.0"

// phraseBonus is subtracted from a candidate's bm25 rank (lower is
// better) when the exact query phrase appears verbatim in its text, so
// an exact-phrase hit outranks a same-terms-any-order hit.
const phraseBonus = 3.0

// candidateFanOut bounds how many raw FTS candidates are pulled per
// distinct page before re-ranking and deduplicating in Go.
const candidateFanOut = 8

// Search runs a warm-path (index-only) search over this site's chunks,
// returning up to limit page-level results ordered best-first. An empty
// or all-punctuation query returns an empty result set, not an error —
// per CLAUDE.md, an empty results array is a normal response.
func (db *DB) Search(query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 10
	}
	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	candidateLimit := limit * candidateFanOut
	if candidateLimit < 50 {
		candidateLimit = 50
	}

	rows, err := db.sql.Query(fmt.Sprintf(`
		SELECT page_url, title, heading_path, text, bm25(chunks_fts, %s) AS rank
		FROM chunks_fts
		WHERE chunks_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?
	`, bm25Weights), ftsQuery, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type candidate struct {
		url, title, headingPath, text string
		rank                          float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.url, &c.title, &c.headingPath, &c.text, &c.rank); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows: %w", err)
	}

	normalizedQuery := strings.ToLower(strings.Join(tokenize(query), " "))

	type scored struct {
		candidate
		adjRank float64
	}
	adjusted := make([]scored, len(candidates))
	for i, c := range candidates {
		adj := c.rank
		if normalizedQuery != "" {
			hay := strings.ToLower(c.title + " " + c.headingPath + " " + c.text)
			if strings.Contains(hay, normalizedQuery) {
				adj -= phraseBonus
			}
		}
		adjusted[i] = scored{candidate: c, adjRank: adj}
	}
	sort.SliceStable(adjusted, func(i, j int) bool { return adjusted[i].adjRank < adjusted[j].adjRank })

	var results []Result
	seen := make(map[string]bool, limit)
	for _, c := range adjusted {
		if seen[c.url] {
			continue
		}
		seen[c.url] = true
		results = append(results, Result{
			URL:         c.url,
			Title:       c.title,
			HeadingPath: c.headingPath,
			Snippet:     snippet(c.text, 200),
			Score:       scoreFromRank(c.adjRank),
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// sanitizeFTSQuery turns free-form user input into a safe FTS5 MATCH
// expression: each token is individually double-quoted (so FTS5 treats it
// as a literal, not a query operator) and space-joined, which FTS5
// interprets as an implicit AND — every token must appear for a chunk to
// match. Quoting also means punctuation/operators in the input (AND, OR,
// *, -, :, unbalanced quotes, ...) can't produce a syntax error.
func sanitizeFTSQuery(q string) string {
	tokens := tokenize(q)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(parts, " ")
}

// tokenize splits s into runs of unicode letters/digits, discarding
// everything else. This intentionally mirrors, at a basic level, what the
// unicode61 tokenizer does at index time, so the phrase-bonus substring
// check and the FTS query itself are built from comparable tokens.
func tokenize(s string) []string {
	var tokens []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, string(cur))
			cur = nil
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func snippet(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	r := []rune(text)
	if len(r) <= maxLen {
		return text
	}
	cut := maxLen
	for cut > 0 && !unicode.IsSpace(r[cut]) {
		cut--
	}
	if cut == 0 {
		cut = maxLen
	}
	return strings.TrimSpace(string(r[:cut])) + "…"
}

// scoreFromRank folds a bm25-derived rank (lower is better, often
// negative for a good match) into an approximate 0..1 display score.
// This is a heuristic for human-facing output, not a calibrated
// probability.
func scoreFromRank(rank float64) float64 {
	d := -rank
	if d < 0 {
		d = 0
	}
	return d / (1 + d)
}
