# Contributing to sitedex

## Dev setup

```sh
git clone https://github.com/Xara-AI/sitedex.git
cd sitedex
make build
make test
make lint
```

Requires Go 1.27+ and no other local tooling — no CGO, no headless
browser, no external services.

## Before opening a PR

- `gofmt`, `go vet`, and `golangci-lint run` must be clean.
- `go test -race ./...` must pass.
- New dependencies are a big deal in this repo (see `CLAUDE.md`'s minimal
  dependencies constraint) — justify any addition to `go.mod` in the PR
  description.
- Keep the repo Xara-agnostic: no product/company-specific concepts in
  code, comments, tests, or docs, outside the single README footer line.

## Adding a platform detector

The CSS-heuristics tier of the product extraction chain
(`internal/extract/product`) is designed as a community-contribution
surface: each e-commerce platform gets a small detector behind a shared
interface, tried only after JSON-LD, microdata, and OpenGraph have all
failed to identify a product on the page. WooCommerce, Shopify, PrestaShop,
and OpenCart ship today (`heuristics_*.go`); Magento and others are welcome
additions.

The interface:

```go
type Detector interface {
	Name() string
	Detect(doc *html.Node, pageURL *url.URL) (p *Product, ok bool)
}
```

Register your detector by adding it to the `Detectors` slice in
`product.go`. A detector should:

1. Check for a reliable marker that you're actually looking at that
   platform's markup before trying to extract anything — a generic class
   name like `.product` isn't enough on its own (see how
   `wooCommerceDetector` requires a `.woocommerce` ancestor, and
   `shopifyDetector` requires a `cdn.shopify.com` script/link tag). A
   detector that fires on someone else's markup is worse than one that
   doesn't fire at all.
2. Return `ok = false` (not an error) when the marker isn't present or a
   product name can't be found — most pages aren't products.
3. Use the shared helpers in `helpers.go` (`findByClass`, `textOfClass`,
   `priceField`, `availabilityField`, `resolveURL`, ...) rather than
   hand-rolling DOM traversal or price parsing.

To add a golden-file fixture: drop a saved (or hand-built, representative)
HTML page under `internal/extract/product/testdata/`, then add a test in
`extract_test.go` asserting the fields your detector should pull out of
it — name, price, currency, availability, image. **Real-world captures
from actual stores (with any personal/customer data stripped) are the most
valuable contributions here** — the synthetic fixtures currently in the
repo are a starting point, not a substitute for how these platforms'
markup varies in practice. This corpus is the tool's real moat; growing it
is the main ongoing contribution surface.

## Project structure and roadmap

See `CLAUDE.md` at the repo root for the full architecture, CLI/HTTP API
contracts, configuration schema, and the milestone build order (M1–M7).
