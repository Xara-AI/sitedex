# sitedex

Turn any website into a clean, chunked, RAG-ready knowledge base — and a
fast product/content search API — with one binary.

**Status: early development.** `crawl`, `search`, `sites`, and `export`
(both `md` and `jsonl`) all work today: point `sitedex crawl` at a site and
it respects robots.txt, discovers pages via sitemap.xml and links, writes
clean per-page markdown (with YAML frontmatter and a heading outline) into
`<data_dir>/<site>/kb/`, and indexes it into a per-site SQLite FTS5 index
for full-text search — including Romanian-diacritic-insensitive matching.
Every crawled page is also run through a product extraction chain
(JSON-LD → microdata → OpenGraph → per-platform CSS heuristics for
WooCommerce/Shopify/PrestaShop/OpenCart); pages that are products show up
in search results with price, currency, and availability. The live HTTP
API and `--fresh` live-verification are not implemented yet — see
`sitedex <command> -h` output, which says exactly what's missing and which
milestone lands it. This note will be replaced with a full README
(quickstart, API reference, extraction chain, platform support table,
limitations) once the HTTP API lands.

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
