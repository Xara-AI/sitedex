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
surface: each e-commerce platform (WooCommerce, Shopify, PrestaShop,
OpenCart, Magento, ...) gets a small detector behind a shared interface.
This lands in milestone M4 — once that package exists, this section will
be expanded with the interface signature, a worked example, and how to add
a golden-file fixture under `testdata/`.

## Project structure and roadmap

See `CLAUDE.md` at the repo root for the full architecture, CLI/HTTP API
contracts, configuration schema, and the milestone build order (M1–M7).
