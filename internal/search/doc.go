// Package search turns a query into ranked results via three paths: warm
// (FTS5 BM25 over the local index), fresh (concurrent live re-fetch and
// re-verification of top results within a response time budget), and cold
// (platform search-endpoint detection for a not-yet-indexed site, with
// background auto-indexing).
//
// Target milestones: M3 (warm path), M5 (fresh path), M6 (cold path). See
// CLAUDE.md, "Search".
package search
