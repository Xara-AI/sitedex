// Package product implements the product extraction priority chain: JSON-LD
// -> microdata/RDFa -> OpenGraph product tags -> per-platform CSS
// heuristics -> optional LLM extractor (disabled by default), stopping at
// first success. Every extracted product records its extraction_method.
//
// Target milestone: M4. See CLAUDE.md, "Product extraction".
package product
