# sitedex

Turn any website into a clean, chunked, RAG-ready knowledge base — and a
fast product/content search API — with one binary.

```sh
$ sitedex crawl --site https://example.com
crawl complete: site=example.com visited=1 fetched=1 skipped=0 failed=0 duration=4ms

$ sitedex search --site example.com --query "blue running shoes"
1. [product] Blue Running Shoes  (score 0.83)
   https://example.com/
   349.99 RON, in_stock (via json-ld)
```

That's a real, unedited transcript (against a one-page test fixture) —
`sitedex serve`'s `POST /v1/search` returns the same result as JSON
instead (see [HTTP API reference](#http-api-reference-sitedex-serve)
below). No demo GIF yet; this transcript is the honest current
substitute — a PR adding one is welcome.

## Why

Most "turn a website into an LLM-ready dataset" tooling either does naive
text scraping (nav menus and cookie banners end up in your chunks right
alongside the actual content) or requires a full headless-browser pipeline
just to read a product's name and price. But most e-commerce and content
sites already publish clean, structured data in their HTML — JSON-LD,
microdata, OpenGraph — because Google's rich-results and social-preview
crawlers read it. sitedex reads the same structured data, strips real
boilerplate with DOM heuristics (not an LLM), and chunks what's left with
heading breadcrumbs intact, so what comes out the other end is usable
context, not noise. And because product data is already there in
machine-readable form on most storefronts, product search comes for free
alongside the content pipeline — no scraping product cards by hand.

## Quickstart

```sh
sitedex crawl --site https://example.com     # crawl -> clean markdown + a searchable index
sitedex search --site example.com --query "blue nike shoes"
sitedex serve                                # HTTP API over everything you've crawled
```

Everything lives under one directory (default `./sitedex-data/`):
`<site>/kb/*.md` (the clean markdown, one file per page, with YAML
frontmatter and a heading outline) and `<site>/index.db` (a SQLite FTS5
index). Backup is `cp -r sitedex-data/ backup/`. There's no database
server, no separate vector store, no state anywhere else.

## Running it

### Container / LXC (the way we run it)

```sh
docker build -t sitedex .
docker run -d -p 8080:8080 -v sitedex-data:/data sitedex
```

`SITEDEX_DATA_DIR` is preset to `/data` in the image; mount a volume there
and everything sitedex knows persists across restarts. The image is
~30MB total (Alpine base plus the pure-Go, no-CGO ~12MB binary) — see
[`Dockerfile`](Dockerfile).

### Bare host / systemd

```sh
sudo cp sitedex /usr/local/bin/sitedex
sudo mkdir -p /etc/sitedex && sudo cp sitedex.example.yaml /etc/sitedex/sitedex.yaml  # then edit it
sudo cp deploy/sitedex.service /etc/systemd/system/sitedex.service
sudo systemctl daemon-reload && sudo systemctl enable --now sitedex
```

See [`deploy/sitedex.service`](deploy/sitedex.service) for the full unit
(it creates a `sitedex` system user, sandboxes the process, and gives
graceful shutdown room to finish in-flight requests and save crawl
state). Logs are structured JSON lines to stdout — `journalctl -u sitedex
-f` or your container platform's log driver, no log files to manage.

## CLI reference

```
sitedex crawl   --site https://example.com [--config sitedex.yaml]
sitedex export  --site example.com --format md|jsonl --out ./kb/ [--config sitedex.yaml]
sitedex search  --site example.com --query "blue nike shoes" [--fresh] [--soft] [--limit 10] [--config sitedex.yaml]
sitedex serve   [--addr :8080] [--config sitedex.yaml]
sitedex sites   [--config sitedex.yaml]
sitedex version
```

Every command also takes `-h`. See [`sitedex.example.yaml`](sitedex.example.yaml)
for the full configuration schema — every key is also settable via a
`SITEDEX_*` environment variable (env always wins over the file), which is
the natural override surface for containers.

## HTTP API reference (`sitedex serve`)

```
POST /v1/search
  {"site":"example.com","query":"blue nike shoes","limit":10,"fresh":true,"type":"product|page|any","soft":false}
→ 200 {"results":[{
     "title": "...", "url": "...", "type": "product|page",
     "price": 199.90, "currency": "RON", "availability": "in_stock|out_of_stock|unknown",
     "image": "...", "snippet": "...", "score": 0.87,
     "verified_at": "2026-08-24T10:00:00Z"   // present only when fresh-verified
   }],
   "source": "index | index+live | site-search",
   "took_ms": 240}

POST /v1/crawl        {"site":"https://example.com"}   → 202 {"job_id":"job-1"}
GET  /v1/crawl/{job}                                    → job status + result once done
GET  /v1/sites                                           → indexed sites, doc counts, last crawl,
                                                             extraction-method breakdown
GET  /v1/sites/{site}/items?since_seq=0&limit=200&type=product
                                                          → what's currently indexed and when each
                                                             item was last touched — a changefeed, see below
GET  /healthz                                            → 200 {"status":"ok"}
GET  /metrics                                             → Prometheus text exposition
```

An empty `results` array is a normal response, never an error. Auth is a
single optional bearer token (`token` in config, or `SITEDEX_TOKEN`) —
absent means open, which is the point for localhost/LXC-internal use;
`/healthz` and `/metrics` are never gated, for probes and scrapers.
`fresh:true` requests are bounded by `search.fresh_timeout_ms` (default
2500ms): if live re-verification of the top results hasn't finished in
time, you get the indexed data back with `verified_at` omitted, not a
blocked request. Searching a site that's never been crawled falls back to
a live on-site search (`source: "site-search"`) instead of empty results,
and — if `search.auto_index_on_cold_query` is enabled (the default in
serve mode) — kicks off a background crawl so the next query is warm.
`soft:true` opts into a bounded fallback for grammatically inflected
queries: if the exact-token search comes back empty, it retries with
query terms suffix-relaxed into prefix matches (e.g. a query ending in
an inflected "-ă" still matches an indexed base form) before giving up.
Off by default; `sitedex search --soft` is the CLI equivalent.

`GET /v1/sites/{site}/items` is how you see what's actually in the index
without guessing from search results: one entry per crawled URL (product
fields layered on when the page has extracted product data), each with a
`seq`. Pass the highest `seq` you've seen back as `since_seq` and you get
only what changed since — a re-crawl or a fresh-verify — so a caller that
polls regularly can stay in sync without re-fetching the whole site every
time. `since_seq=0` (or omitted) returns everything.

## How extraction works

Every crawled page runs through a priority chain, stopping at the first
tier that succeeds:

1. **JSON-LD** (`<script type="application/ld+json">`) — `Product`,
   `Offer`, `AggregateOffer`, `ItemList`. Parsed tolerantly: arrays,
   `@graph`, nested/multiple offers, price as a localized string
   (`"1.234,56"` and `"1,234.56"` both work).
2. **Microdata** (`itemtype=".../Product"` + a nested `.../Offer`).
3. **OpenGraph** product tags (`og:type=product`, `product:price:amount`).
4. **CSS heuristics**, one small detector per platform, gated on a
   platform-specific marker so a generic `.product` div on someone else's
   site can't false-positive:

   | Platform | Detected by |
   |---|---|
   | WooCommerce | a `.woocommerce` ancestor |
   | Shopify | a `cdn.shopify.com` script/link tag |
   | PrestaShop | `<meta name="generator" content="PrestaShop">` |
   | OpenCart | `<meta name="generator" content="OpenCart">` |

   Adding a platform is the main community-contribution surface here —
   see [`CONTRIBUTING.md`](CONTRIBUTING.md#adding-a-platform-detector).
5. **An optional LLM extractor** (OpenAI or Anthropic) — disabled by
   default, costs money when enabled, and is a last resort for platforms
   sitedex doesn't recognize yet, not the engine. See
   `llm_extractor` in [`sitedex.example.yaml`](sitedex.example.yaml).

Every extracted product records which tier found it (`extraction_method`)
— surfaced in search ranking (JSON-LD ranks above heuristics, which ranks
above the LLM tier) and handy for debugging why a page did or didn't turn
up as a product.

Search-results/category pages (many products per page) get the same
treatment via a list-aware variant: JSON-LD `ItemList` first, then a
generic repeated-card heuristic.

Regular content pages skip all of that and go straight to markdown:
boilerplate (nav/footer/cookie banners) is stripped with DOM heuristics —
text-density scoring, link-density thresholds, semantic tags first — no
LLM involved, then the result is chunked on heading boundaries with a
heading-path breadcrumb (`Shoes > Running`) kept as context on each chunk.
That breadcrumb is what makes the output usable as RAG context instead of
arbitrarily-sliced text.

## Honest limitations

- **No JavaScript rendering.** sitedex reads raw HTML — it doesn't run a
  headless browser. In practice this matters less than it sounds:
  JSON-LD, microdata, and OpenGraph tags almost always survive server-side
  rendering even on heavily client-rendered storefronts, because they're
  there for Google and social-preview crawlers, not for the page's own JS.
  A `renderer_url` config hook exists for plugging in an external
  rendering service later; nothing implements it yet.
- **Polite by default, and that's a real constraint, not just a nicety.**
  robots.txt (crawl-delay included) is respected, the default rate limit
  is 1 request/second/host, and sitedex identifies itself with a real
  User-Agent. All of this is overridable in config — that's for sites you
  own or have explicit permission to crawl, not a bypass.
- **No embeddings, no vector search.** Search is SQLite FTS5 (BM25)
  today. Embeddings are a natural v2 and on the public roadmap, not
  implemented.
- **One host, many sites — not distributed.** `sitedex serve` runs
  everything you've crawled from a single process on a single machine.
  That's the whole design; it's what keeps the "one binary, one data
  directory, `cp -r` to back it all up" story true.
- **Slug collisions on query-string-only page variants.** The markdown
  export keys files by URL path, ignoring the query string — pages that
  differ only by `?query=param` collide onto the same file. Revisit if
  this bites you in practice.

## License

MIT — see [LICENSE](LICENSE).

---

Built and maintained by [Xara](https://xara.bot) — we use sitedex in
production.
