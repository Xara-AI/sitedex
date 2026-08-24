// Package content converts a crawled HTML page into clean markdown with a
// heading outline, stripping boilerplate (nav/footer/aside/cookie banners)
// via DOM heuristics — no LLM involved — and splits that markdown into
// heading-anchored, size-bounded chunks for indexing.
//
// See CLAUDE.md, "Content extraction".
package content
