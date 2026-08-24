# sitedex

Single-binary Go tool that crawls a website into a clean, chunked, RAG-ready
knowledge base **and** serves fast product/content search over it, with live
freshness verification. MIT license. Public repo: `github.com/Xara-AI/sitedex`.

Naming history: working name was `sitekb` during spec drafting (alternatives
considered: `webdex`, `kbcrawl`); the name actually adopted for the repo and
module path is **sitedex**. If you find `sitekb` in older notes/drafts, treat
it as this same project — rename to `sitedex` wherever it appears in code,
CLI examples, config keys (`sitedex.yaml`, `SITEDEX_*` env vars), the Go
module path, and the user-agent string.

## Non-Negotiable Constraints

- **Go, single static binary, no CGO.** Use `modernc.org/sqlite` (pure Go)
  so cross-compilation stays static. Target: binary < 25 MB, idle RSS
  < 50 MB, zero runtime deps on bare Alpine/Debian.
- **No headless browser in v1.** Document honestly that JS-heavy sites won't
  render, but JSON-LD usually survives in raw HTML. Leave a documented
  `renderer_url` config hook for a future external rendering service —
  do not implement one.
- **Polite by default:** respect `robots.txt` (incl. crawl-delay), default
  rate limit 1 req/s/host, identify as `sitedex/<version> (+repo URL)`.
  Overridable via config; README must state overrides are for sites you own
  or have permission to crawl.
- **Minimal dependencies.** Stdlib first. Allowed extras: `golang.org/x/net/html`,
  `modernc.org/sqlite`, one CLI lib (`spf13/cobra` or stdlib `flag` — prefer
  stdlib if ergonomics survive), one YAML parser. Any other dependency needs
  justification in the PR description.
- CI quality gates: `gofmt`, `go vet`, `golangci-lint`, `go test ./...` with
  `-race`. GitHub Actions cross-compiles releases (linux/amd64, linux/arm64,
  darwin/arm64, windows/amd64) on tag.

## CLI Surface

```
sitedex crawl   --site https://example.com [--config sitedex.yaml]   # full crawl -> markdown + index
sitedex export  --site example.com --format md|jsonl --out ./kb/     # dump the knowledge base
sitedex search  --site example.com --query "blue nike shoes" [--fresh] [--limit 10]
sitedex serve   [--addr :8080]                                       # long-running HTTP daemon
sitedex sites                                                         # list indexed sites + stats
sitedex version
```

- `crawl` = batch mode (crawl → export → done, no daemon required).
- `serve` = live mode: HTTP API over all indexed sites, background re-crawls
  on schedule.
- Data dir (default `./sitedex-data/`): `<site>/kb/*.md` + `<site>/index.db`
  (SQLite). Backup = copy the dir. Document this as a deliberate simplicity
  feature.

## HTTP API (serve mode)

```
POST /v1/search
  {"site":"example.com","query":"blue nike shoes","limit":10,"fresh":true,"type":"product|page|any"}
→ 200 {"results":[{
     "title": "...", "url": "...", "type": "product|page",
     "price": 199.90, "currency": "RON", "availability": "in_stock|out_of_stock|unknown",
     "image": "...", "snippet": "...", "score": 0.87,
     "verified_at": "2026-08-24T10:00:00Z"   // present only when fresh-verified
   }],
   "source": "index|index+live|site-search",
   "took_ms": 240}

POST /v1/crawl        {"site":"https://example.com"}        → 202 {"job_id":...}
GET  /v1/crawl/{job}                                        → job status/progress
GET  /v1/sites                                               → indexed sites, doc counts, last crawl
GET  /healthz                                                → 200 ok
```

- Empty `results` array is a valid, normal response — never an error.
- Auth: single optional bearer token via config/env (`SITEDEX_TOKEN`). Absent
  = open (localhost/LXC-internal use case). Keep it this simple.
- Hard response budget: `fresh:true` requests must return within
  `fresh_timeout` (default 2500 ms) — if live verification hasn't finished,
  return index data with `verified_at` omitted rather than blocking. This
  bounded-latency guarantee is a core requirement, not a nice-to-have: the
  API needs to be safe to call from a live, latency-sensitive request path
  (a search box, an assistant's tool call, any synchronous UI), not just
  batch/offline consumers.

## Pipeline Architecture

```
internal/
├── crawler/      # frontier, robots.txt, rate limiter, fetcher
├── extract/      # html -> structured content (the heart of the tool)
│   ├── product/  #   product extraction priority chain (see below)
│   └── content/  #   article/page -> clean markdown + chunking
├── index/        # SQLite FTS5 index, upserts keyed by URL
├── search/       # query -> ranked results (+ live verify, + cold path)
├── export/       # markdown / JSONL emitters
├── server/       # HTTP API
└── config/       # yaml + env loading
```

### Crawl
- BFS from root + sitemap.xml (parse if present).
- Same-registrable-domain only. Include/exclude URL regex patterns in
  config. Max pages default 2000, max depth default 5.
- Conditional revalidation on re-crawl: store ETag/Last-Modified, send
  If-None-Match/If-Modified-Since; unchanged pages skip extraction.
- Content hash per page; unchanged hash = no index write.

### Content extraction (`extract/content`)
- Strip boilerplate (nav/footer/aside/cookie banners) via DOM heuristics:
  text-density scoring, link-density thresholds, semantic tags (`<main>`,
  `<article>`) preferred first. No LLM.
- Emit clean markdown per page with YAML frontmatter: `url`, `title`,
  `description`, `lang`, `crawled_at`, `hash`, `headings` outline.
- Chunking: split on heading boundaries, target chunk size configurable
  (default ~1200 chars, overlap 100). Chunks carry heading-path breadcrumb
  (`H1 > H2`) as metadata — this is what makes output genuinely RAG-ready
  vs. naive splitting.

### Product extraction (`extract/product`) — priority chain, stop at first success
1. **JSON-LD** `Product`/`ItemList`/`Offer` — covers most WooCommerce/
   Shopify/PrestaShop/Magento/OpenCart sites. Parse tolerantly (arrays,
   `@graph`, nested offers, price as string).
2. **Microdata/RDFa** (`itemtype*=schema.org/Product`).
3. **OpenGraph product tags** (`og:type=product`, `product:price:amount`).
4. **CSS heuristics** — small per-platform detectors behind a shared
   interface (WooCommerce `.product`+`.price`, etc.) — this is the
   designed community-contribution surface; say so in CONTRIBUTING.md.
5. **Optional LLM extractor** — disabled by default; config
   `llm_extractor: {provider: openai|anthropic|none, model, api_key_env}`.
   Stripped HTML in, strict-schema product JSON out. Marked experimental +
   costs money. Pluggable last resort, not the engine.
- Every extracted product records `extraction_method`, surfaced in search
  results via score weighting (JSON-LD > heuristics > llm) and used for
  debugging.

### Index (`index/`)
- SQLite, FTS5 (unicode61 tokenizer, `remove_diacritics 2` — Romanian
  diacritics must match unaccented queries: "pantofi" matches "pantofi",
  "PANTOFI", and diacritic variants).
- Tables: `pages(url PK, title, lang, hash, crawled_at, etag, ...)`,
  `chunks(page_url, ord, heading_path, text)` + FTS mirror,
  `products(page_url PK, name, price, currency, availability, image,
  raw_json)` + FTS mirror on name+description.
- Ranking: BM25 (FTS5 built-in) with boosts: title match > product-name
  match > body; exact-phrase bonus; product-type filter when
  `type:"product"`.

### Search (`search/`)
- **Warm path:** query → FTS5 → ranked top N (ms-fast).
- **Fresh verify (`fresh:true`):** concurrently re-fetch top `fresh_top_n`
  (default 3) result URLs with per-request timeout (default 1500 ms),
  re-run product extraction, update price/availability in response + index.
  Overall budget guard per the API section above.
- **Cold path** (site requested but not indexed, or `source:"site-search"`
  forced): platform-detect the site's own search endpoint (WooCommerce
  `/?s=`, Shopify `/search?q=`, PrestaShop `/recherche?s=`/`search?controller=search&s=`,
  OpenCart `index.php?route=product/search&search=`, generic
  `<form role=search>` discovery) → fetch results page → run product
  extraction chain on it → return with `source:"site-search"`. Then enqueue
  a background full crawl of that site (config flag
  `auto_index_on_cold_query`, default true in serve mode) so the next query
  is warm.

## Configuration

`sitedex.yaml` (all overridable via `SITEDEX_*` env vars — env-first for
container use):

```yaml
data_dir: ./sitedex-data
listen: ":8080"
token: ""                      # empty = no auth
crawl:
  rate_limit_rps: 1.0
  max_pages: 2000
  max_depth: 5
  respect_robots: true         # override only for sites you own
  user_agent: "sitedex/{version} (+https://github.com/Xara-AI/sitedex)"
  include: []                  # URL regex allowlist (empty = all)
  exclude: ["/cart", "/checkout", "/wp-admin"]
  recrawl_interval: 24h        # serve mode background refresh
search:
  fresh_top_n: 3
  fresh_timeout_ms: 2500
  auto_index_on_cold_query: true
chunking:
  target_chars: 1200
  overlap_chars: 100
llm_extractor:
  provider: none                # none|openai|anthropic
```

## Deployment (LXC / infra)

- README ships a 10-line "run it in a container" section: download binary →
  `sitedex serve` → done. Provide a sample `systemd` unit and a sample
  `Dockerfile` (FROM alpine, COPY binary, ~20 MB image) — Docker is for the
  community; binary+systemd is the LXC story.
- Data dir on a mounted volume; state = that one dir.
- Graceful shutdown: finish in-flight requests, checkpoint crawl frontier so
  an interrupted crawl resumes.
- Structured logs to stdout (JSON lines, level via config) —
  journald/docker-logs friendly, no log files.
- Prometheus-style `/metrics` (crawl counts, index sizes, search latency
  histogram).

## Testing Requirements

- Extraction chain: golden-file tests — `testdata/` tree of real-world saved
  HTML pages per platform (WooCommerce, Shopify, PrestaShop, OpenCart,
  JSON-LD variants, malformed JSON-LD) → expected product JSON. **This
  corpus is the tool's real moat** — growing it is the main ongoing dev
  activity.
- Crawler: tests against a local `httptest` server (robots rules, rate
  limiting, redirect loops, ETag revalidation).
- Search: ranking regression tests (fixed corpus, assert top-3 ordering for
  representative queries incl. Romanian diacritics).
- Fresh-path: timeout behavior test (slow upstream must not break the
  response budget).

## README Skeleton (also the marketing surface)

1. One-liner + 30-second demo GIF (crawl a shop → search "blue shoes" →
   JSON with prices).
2. "Why": RAG projects need clean site data; naive scraping produces
   garbage chunks; product data is already structured on most sites — use
   it.
3. Quickstart (3 commands). LXC/Docker/systemd section.
4. API reference (the HTTP API section above).
5. Extraction chain explained + supported platforms table (with "add
   yours — see CONTRIBUTING").
6. Honest limitations: no JS rendering (and why it usually doesn't matter),
   politeness defaults.
7. Footer: "Built and maintained by [Xara](https://xara.bot) — we use
   sitedex in production." Single funnel line, nothing more.

## Build Order (sequential milestones)

Each milestone is independently reviewable and mergeable.

1. **M1:** repo scaffolding, config, CLI skeleton, CI (lint+test+release
   workflow).
2. **M2:** crawler (robots, rate limit, sitemap, frontier, revalidation) +
   content extraction + markdown export. *At this point the tool is already
   useful — "site → clean markdown" alone is shippable.*
3. **M3:** SQLite FTS5 index + `search` CLI + ranking tests (incl.
   diacritics).
4. **M4:** product extraction chain (JSON-LD → microdata → OG → heuristics)
   + golden-file corpus.
5. **M5:** `serve` HTTP API + fresh-verify path + metrics + graceful
   shutdown.
6. **M6:** cold path (platform search-endpoint detection) +
   auto-index-on-cold-query.
7. **M7:** optional LLM extractor plugin + docs polish + README + release
   v0.1.0.

M2 is the minimal public-launch candidate if the repo opens early and builds
in public.

## Explicitly Out of Scope (v1)

- JS rendering / headless browsers (hook only, via `renderer_url`).
- Embeddings / vector search (FTS5 only for v1; embeddings are a natural v2
  and a good public roadmap item).
- Multi-tenant auth, user accounts, any UI beyond the API.
- Distributed crawling; one binary, one host, many sites is the model.
- Any specific downstream product integration — consuming this API from
  another application is that application's concern, not this repo's;
  sitedex is a general-purpose tool, not built around one integration.
