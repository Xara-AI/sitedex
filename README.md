# sitedex

Turn any website into a clean, chunked, RAG-ready knowledge base — and a
fast product/content search API — with one binary.

**Status: early development, but functionally complete for single-site
crawl/index/search.** `crawl`, `search`, `sites`, `export` (both `md` and
`jsonl`), and `serve` all work today. `sitedex crawl` respects robots.txt,
discovers pages via sitemap.xml and links, writes clean per-page markdown
(YAML frontmatter + heading outline) into `<data_dir>/<site>/kb/`, and
indexes it into a per-site SQLite FTS5 index — including
Romanian-diacritic-insensitive matching. Every crawled page also runs
through a product extraction chain (JSON-LD → microdata → OpenGraph →
per-platform CSS heuristics for WooCommerce/Shopify/PrestaShop/OpenCart),
so product pages show up in search results with price, currency, and
availability. `sitedex serve` runs the HTTP API (`/v1/search` — including
`fresh:true` live re-verification within a hard response-time budget,
`/v1/crawl`, `/v1/sites`, `/healthz`, `/metrics`), a background re-crawl
scheduler, structured JSON logging, and graceful shutdown.

Not implemented yet: the "cold path" (searching/auto-indexing a site
that's never been crawled) and the optional LLM extraction fallback — see
`sitedex <command> -h` output for what's missing and which milestone lands
it. This note will be replaced with a full README (quickstart, API
reference, extraction chain, platform support table, limitations) closer
to the v0.1.0 release.

## Build

Requires Go 1.27+.

```sh
make build      # -> bin/sitedex
make test       # go test -race ./...
make lint       # golangci-lint
```

## CLI

```sh
sitedex crawl   --site https://example.com [--config sitedex.yaml]
sitedex export  --site example.com --format md|jsonl --out ./kb/
sitedex search  --site example.com --query "blue nike shoes" [--fresh] [--limit 10]
sitedex serve   [--addr :8080]
sitedex sites
sitedex version
```

See `sitedex.example.yaml` for the full configuration schema (every key is
also settable via a `SITEDEX_*` environment variable, which takes
precedence over the config file).

## License

MIT — see [LICENSE](LICENSE).

---

Built and maintained by [Xara](https://xara.bot) — we use sitedex in
production.
