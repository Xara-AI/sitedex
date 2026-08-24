// Package content converts a crawled HTML page into clean markdown with
// YAML frontmatter and heading-anchored chunks, stripping boilerplate
// (nav/footer/aside/cookie banners) via DOM heuristics — no LLM involved.
//
// Target milestone: M2. See CLAUDE.md, "Content extraction".
package content
