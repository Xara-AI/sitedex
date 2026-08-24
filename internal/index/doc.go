// Package index manages the per-site SQLite FTS5 index (pages, chunks,
// products, and their FTS mirrors), keyed by URL, with the unicode61
// tokenizer and remove_diacritics=2 so Romanian diacritics match
// unaccented queries.
//
// Target milestone: M3. See CLAUDE.md, "Index".
package index
