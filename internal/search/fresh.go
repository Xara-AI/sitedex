package search

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/extract/product"
	"github.com/Xara-AI/sitedex/internal/index"
)

// Request is a full search request, as the HTTP API accepts it.
type Request struct {
	Site  string
	Query string
	Limit int
	Type  string // ""/"any", "page", or "product"
	Fresh bool

	// FreshTopN and FreshTimeout configure the fresh-verify path (search.*
	// config); zero values fall back to CLAUDE.md's documented defaults
	// (3 results, 2500ms).
	FreshTopN    int
	FreshTimeout time.Duration
}

// Response is a full search response, as the HTTP API returns it.
type Response struct {
	Results []Result
	// Source is "index" (warm path only) or "index+live" (at least one
	// result was successfully fresh-verified).
	Source string
}

const (
	defaultFreshTopN  = 3
	defaultFreshTopMs = 2500
	// perFetchTimeout bounds a single fresh-verify page fetch — separate
	// from and typically shorter than the overall FreshTimeout budget, so
	// one slow site can't by itself consume the whole budget and starve
	// the others.
	perFetchTimeout = 1500 * time.Millisecond
)

// SearchFresh runs req.Type-filtered warm search, then — if req.Fresh and
// there's at least one result — concurrently re-fetches the top
// req.FreshTopN results and re-runs product extraction on them, updating
// both the response and the index with whatever comes back before
// req.FreshTimeout elapses. Verification that doesn't finish in time is
// simply dropped (those results keep their indexed data, with no
// verified_at), never blocking the response past budget — per CLAUDE.md's
// "Hard response budget" requirement.
//
// Unlike Search (the CLI's entry point), an uncrawled site is not an
// error here: it returns zero results with source "index", matching the
// HTTP API's "empty results is a normal response" contract. This is a
// deliberate stand-in for M6's cold-path fallback, which will replace
// this empty response with a live site-search — the response shape
// won't need to change when that lands.
func (s *Searcher) SearchFresh(ctx context.Context, req Request) (Response, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	dbPath := index.Path(s.dataDir, req.Site)
	if _, err := os.Stat(dbPath); err != nil {
		return Response{Source: "index"}, nil
	}
	idx, err := index.Open(s.dataDir, req.Site)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = idx.Close() }()

	results, err := idx.SearchFiltered(req.Query, limit, normalizeType(req.Type))
	if err != nil {
		return Response{}, err
	}
	resp := Response{Results: results, Source: "index"}
	if !req.Fresh || len(results) == 0 {
		return resp, nil
	}

	if s.verifyTop(ctx, idx, resp.Results, req) {
		resp.Source = "index+live"
	}
	return resp, nil
}

// verifyTop concurrently re-fetches and re-extracts the top N results in
// place (mutating results' Price/Currency/Availability/Image/Verified
// fields) and persists successful verifications to idx. It returns
// whether at least one result was verified.
func (s *Searcher) verifyTop(ctx context.Context, idx *index.DB, results []Result, req Request) bool {
	topN := req.FreshTopN
	if topN <= 0 {
		topN = defaultFreshTopN
	}
	if topN > len(results) {
		topN = len(results)
	}
	timeout := req.FreshTimeout
	if timeout <= 0 {
		timeout = defaultFreshTopMs * time.Millisecond
	}

	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := crawler.NewHTTPClient()

	type verified struct {
		i  int
		p  *product.Product
		ok bool
	}
	// Buffered so every goroutine can always send, even ones we stop
	// listening to after the timeout — no goroutine leak.
	ch := make(chan verified, topN)
	for i := 0; i < topN; i++ {
		go func(i int) {
			u, err := url.Parse(results[i].URL)
			if err != nil {
				ch <- verified{i: i}
				return
			}
			fetchCtx, fetchCancel := context.WithTimeout(fctx, perFetchTimeout)
			defer fetchCancel()
			res, err := crawler.FetchPage(fetchCtx, client, s.userAgent, results[i].URL, "", "")
			if err != nil || res.NotModified || res.StatusCode != http.StatusOK {
				ch <- verified{i: i}
				return
			}
			p, ok := product.Extract(res.Body, u)
			ch <- verified{i: i, p: p, ok: ok}
		}(i)
	}

	verifiedAny := false
	for remaining := topN; remaining > 0; remaining-- {
		select {
		case v := <-ch:
			if v.ok && v.p != nil {
				applyVerified(&results[v.i], v.p)
				_ = idx.IndexProduct(results[v.i].URL, &index.ProductRecord{
					Name: v.p.Name, Description: v.p.Description, Price: v.p.Price, HasPrice: v.p.HasPrice,
					Currency: v.p.Currency, Availability: string(v.p.Availability), Image: v.p.Image,
					ExtractionMethod: string(v.p.ExtractionMethod), RawJSON: v.p.RawJSON,
				})
				verifiedAny = true
			}
		case <-fctx.Done():
			return verifiedAny // budget exceeded: return whatever we verified so far
		}
	}
	return verifiedAny
}

func applyVerified(r *Result, p *product.Product) {
	r.Price = p.Price
	r.HasPrice = p.HasPrice
	r.Currency = p.Currency
	r.Availability = string(p.Availability)
	if p.Image != "" {
		r.Image = p.Image
	}
	r.Verified = true
	r.VerifiedAt = time.Now().UTC()
}

func normalizeType(t string) string {
	switch t {
	case "page", "product":
		return t
	default:
		return "" // "" and "any" (and anything else) mean unfiltered
	}
}
