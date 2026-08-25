package index

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Result is one page-level search hit — either the best-scoring chunk of a
// regular page (Type "page") or an extracted product (Type "product"). A
// page can match on both its content and its product data; when it does,
// the product representation wins (see Search), matching the richer
// product-search use case this tool is built for.
type Result struct {
	URL         string
	Type        string // "page" or "product"
	Title       string
	HeadingPath string // page results only
	Snippet     string

	// Product results only:
	Price            float64
	HasPrice         bool
	Currency         string
	Availability     string
	Image            string
	ExtractionMethod string

	Score float64 // heuristic, roughly 0..1, higher is better

	// Verified and VerifiedAt are set by the search package's fresh-verify
	// path (M5), not by Search itself — included here so index.Result can
	// carry the full result shape end to end without a parallel DTO.
	Verified   bool
	VerifiedAt time.Time
}

// chunkBM25Weights assigns column weights for chunks_fts (page_url, ord,
// title, heading_path, text, in schema order): title matches rank well
// above heading-path matches, which rank above plain body text. page_url/
// ord are UNINDEXED and never match, so their weight is irrelevant.
const chunkBM25Weights = "0, 0, 5.0, 2.0, 1.0"

// productBM25Weights assigns column weights for products_fts (page_url,
// name, description, in schema order): name matches rank above
// description matches.
const productBM25Weights = "0, 3.0, 1.0"

// productMethodBonus is subtracted from a product candidate's bm25 rank
// (lower is better) based on how it was extracted, so a JSON-LD-sourced
// product outranks an equally-relevant heuristic-sourced one — per
// CLAUDE.md: "JSON-LD > heuristics > llm".
var productMethodBonus = map[string]float64{
	"json-ld":   2.0,
	"microdata": 1.5,
	"opengraph": 1.0,
	"heuristic": 0.0,
	"llm":       -1.0,
}

// phraseBonus is subtracted from a candidate's bm25 rank (lower is
// better) when the exact query phrase appears verbatim in its matched
// text, so an exact-phrase hit outranks a same-terms-any-order hit.
const phraseBonus = 3.0

// candidateFanOut bounds how many raw FTS candidates are pulled per
// distinct page before re-ranking and deduplicating in Go.
const candidateFanOut = 8

// Search runs a warm-path (index-only) search over this site's pages and
// products, returning up to limit results ordered best-first. An empty or
// all-punctuation query returns an empty result set, not an error — per
// CLAUDE.md, an empty results array is a normal response.
func (db *DB) Search(query string, limit int) ([]Result, error) {
	return db.SearchFiltered(query, limit, "")
}

// SearchFiltered is Search with an optional type filter: "" or "any"
// searches both pages and products (Search's behavior), "page" or
// "product" searches only that type. Filtering here (rather than after
// Search truncates to limit) ensures a type-filtered request still gets a
// full `limit` results when that many exist.
func (db *DB) SearchFiltered(query string, limit int, typeFilter string) ([]Result, error) {
	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	normalizedQuery := strings.ToLower(strings.Join(tokenize(query), " "))
	return db.searchByFTSQuery(ftsQuery, normalizedQuery, limit, typeFilter)
}

// minSoftStemLen is the shortest a token may be truncated down to before
// SearchSoft leaves it as an exact match instead of loosening it into a
// prefix query — stops a short word like "and" or "și" from turning into
// a near-open wildcard.
const minSoftStemLen = 4

// maxSoftSuffixTrim bounds how many trailing runes SearchSoft will try
// stripping before giving up on a token. 3 covers the common Romanian
// inflectional endings (gender/number/case agreement — e.g. the indexed
// "externalizat" vs. a query's "externalizată"/"externalizate") without
// the query degrading into an open-ended fuzzy search.
const maxSoftSuffixTrim = 3

// SearchSoft is SearchFiltered with a bounded suffix-relaxation fallback:
// when the strict exact-token query comes back with zero results, it
// retries up to maxSoftSuffixTrim times, each round truncating every
// sufficiently long query token by one more trailing rune and turning it
// into an FTS5 prefix query. Every term is still required (AND semantics)
// — this loosens *how* a term is allowed to match, not *whether* it must
// appear, so a relaxed hit still reflects the whole query rather than a
// subset of it (unlike dropping a term outright).
//
// This exists for grammatically inflected queries: Romanian marks
// gender/number/case agreement on nouns and adjectives, so a visitor's
// own wording ("echipă marketing externalizată") can differ from a site's
// copy ("echipă de marketing externalizat") by a single trailing letter
// yet match nothing under exact-token search. It's opt-in (not run by
// SearchFiltered/Search) because prefix matching is inherently looser: a
// caller should ask for it deliberately, e.g. after a strict search
// already came back empty.
func (db *DB) SearchSoft(query string, limit int, typeFilter string) ([]Result, error) {
	results, err := db.SearchFiltered(query, limit, typeFilter)
	if err != nil || len(results) > 0 {
		return results, err
	}

	normalizedQuery := strings.ToLower(strings.Join(tokenize(query), " "))
	for trim := 1; trim <= maxSoftSuffixTrim; trim++ {
		relaxedQuery := relaxFTSQuery(query, trim)
		if relaxedQuery == "" {
			break
		}
		relaxed, err := db.searchByFTSQuery(relaxedQuery, normalizedQuery, limit, typeFilter)
		if err != nil {
			return nil, err
		}
		if len(relaxed) > 0 {
			return relaxed, nil
		}
	}
	return results, nil // still empty: nil per "empty results is normal", not an error
}

// searchByFTSQuery runs ftsQuery (already a complete FTS5 MATCH
// expression) against products and/or chunks per typeFilter, merges,
// dedupes (product representation wins — see Result), ranks by Score, and
// truncates to limit. Shared by SearchFiltered (exact) and SearchSoft
// (suffix-relaxed) so both go through identical ranking/merge logic.
func (db *DB) searchByFTSQuery(ftsQuery, normalizedQuery string, limit int, typeFilter string) ([]Result, error) {
	if limit <= 0 {
		limit = 10
	}
	candidateLimit := limit * candidateFanOut
	if candidateLimit < 50 {
		candidateLimit = 50
	}

	var productResults, chunkResults []Result
	var err error
	if typeFilter != "page" {
		productResults, err = db.searchProducts(ftsQuery, candidateLimit, normalizedQuery)
		if err != nil {
			return nil, err
		}
	}
	if typeFilter != "product" {
		chunkResults, err = db.searchChunks(ftsQuery, candidateLimit, normalizedQuery)
		if err != nil {
			return nil, err
		}
	}

	var results []Result
	seen := make(map[string]bool, limit)
	// Products first: for a page that matches on both its product data and
	// its content, prefer showing it as a product (richer, more useful
	// result for this tool's purpose).
	for _, r := range productResults {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		results = append(results, r)
	}
	for _, r := range chunkResults {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		results = append(results, r)
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (db *DB) searchChunks(ftsQuery string, candidateLimit int, normalizedQuery string) ([]Result, error) {
	rows, err := db.sql.Query(fmt.Sprintf(`
		SELECT page_url, title, heading_path, text, bm25(chunks_fts, %s) AS rank
		FROM chunks_fts
		WHERE chunks_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?
	`, chunkBM25Weights), ftsQuery, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
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
			return nil, fmt.Errorf("scan chunk row: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chunk rows: %w", err)
	}

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

	seen := make(map[string]bool)
	var out []Result
	for _, c := range adjusted {
		if seen[c.url] {
			continue
		}
		seen[c.url] = true
		out = append(out, Result{
			URL: c.url, Type: "page", Title: c.title, HeadingPath: c.headingPath,
			Snippet: snippet(c.text, 200), Score: scoreFromRank(c.adjRank),
		})
	}
	return out, nil
}

func (db *DB) searchProducts(ftsQuery string, candidateLimit int, normalizedQuery string) ([]Result, error) {
	rows, err := db.sql.Query(fmt.Sprintf(`
		SELECT pf.page_url, p.name, p.description, p.price, p.currency, p.availability, p.image, p.extraction_method,
		       bm25(products_fts, %s) AS rank
		FROM products_fts pf
		JOIN products p ON p.page_url = pf.page_url
		WHERE products_fts MATCH ?
		ORDER BY rank ASC
		LIMIT ?
	`, productBM25Weights), ftsQuery, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type candidate struct {
		url, name, description, currency, availability, image, method string
		price                                                         sql.NullFloat64
		rank                                                          float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.url, &c.name, &c.description, &c.price, &c.currency, &c.availability, &c.image, &c.method, &c.rank); err != nil {
			return nil, fmt.Errorf("scan product row: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("product rows: %w", err)
	}

	type scored struct {
		candidate
		adjRank float64
	}
	adjusted := make([]scored, len(candidates))
	for i, c := range candidates {
		adj := c.rank - productMethodBonus[c.method]
		if normalizedQuery != "" {
			hay := strings.ToLower(c.name + " " + c.description)
			if strings.Contains(hay, normalizedQuery) {
				adj -= phraseBonus
			}
		}
		adjusted[i] = scored{candidate: c, adjRank: adj}
	}
	sort.SliceStable(adjusted, func(i, j int) bool { return adjusted[i].adjRank < adjusted[j].adjRank })

	seen := make(map[string]bool)
	var out []Result
	for _, c := range adjusted {
		if seen[c.url] {
			continue
		}
		seen[c.url] = true
		out = append(out, Result{
			URL: c.url, Type: "product", Title: c.name, Snippet: snippet(c.description, 200),
			Price: c.price.Float64, HasPrice: c.price.Valid, Currency: c.currency,
			Availability: c.availability, Image: c.image, ExtractionMethod: c.method,
			Score: scoreFromRank(c.adjRank),
		})
	}
	return out, nil
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

// relaxFTSQuery rebuilds q's FTS5 MATCH expression for SearchSoft: each
// token long enough to spare it (len - trim >= minSoftStemLen) is
// truncated by trim trailing runes and turned into an unquoted FTS5
// prefix query ("externalizat*"), so it matches any indexed word sharing
// that stem — covering the trimmed token itself and any of its own
// suffixed forms. Tokens too short to trim safely keep their exact
// quoted form, same as sanitizeFTSQuery. Unquoted is safe here because
// tokenize already restricted every token to letters/digits, so there's
// nothing to escape and no FTS5 operator can sneak in. Returns "" if q
// tokenizes to nothing.
func relaxFTSQuery(q string, trim int) string {
	tokens := tokenize(q)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		r := []rune(t)
		if len(r)-trim >= minSoftStemLen {
			parts[i] = string(r[:len(r)-trim]) + "*"
		} else {
			parts[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		}
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
