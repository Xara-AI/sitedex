// Package server implements the sitedex HTTP API (POST /v1/search, POST
// /v1/crawl, GET /v1/crawl/{job}, GET /v1/sites, GET /healthz, GET
// /metrics) for "sitedex serve" mode, including the fresh-verify response
// budget and graceful shutdown.
//
// Target milestone: M5. See CLAUDE.md, "HTTP API (serve mode)".
package server
