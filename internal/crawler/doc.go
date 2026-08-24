// Package crawler implements the sitedex crawl frontier: robots.txt
// handling, per-host rate limiting, sitemap.xml discovery, BFS traversal,
// and conditional (ETag/Last-Modified) revalidation on re-crawl.
//
// See CLAUDE.md, "Crawl".
package crawler
