# sitedex

Turn any website into a clean, chunked, RAG-ready knowledge base — and a
fast product/content search API — with one binary.

**Status: early development.** `crawl` and `export --format md` work today:
point `sitedex crawl` at a site and it respects robots.txt, discovers pages
via sitemap.xml and links, and writes clean per-page markdown (with YAML
frontmatter and a heading outline) into `<data_dir>/<site>/kb/`. Indexing,
search, product extraction, and the HTTP API are not implemented yet — see
`sitedex <command> -h` output, which says exactly what's missing and which
milestone lands it. This note will be replaced with a full README
(quickstart, API reference, extraction chain, platform support table,
limitations) once search and product extraction land.

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
