package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxRedirects caps automatic redirect following. The stdlib default (10)
// would eventually break a genuine redirect loop too, but with a much
// less useful error and a much longer wait; we fail fast and clearly.
const maxRedirects = 5

// defaultFetchTimeout bounds a single page fetch (including any
// redirects). It is intentionally not user-configurable in v1 — it is an
// implementation detail, not a politeness knob like rate_limit_rps.
const defaultFetchTimeout = 20 * time.Second

// maxBodyBytes caps how much of a response body is read, protecting
// against unexpectedly huge pages.
const maxBodyBytes = 20 * 1024 * 1024

// ErrTooManyRedirects is returned when a fetch exceeds maxRedirects.
var ErrTooManyRedirects = errors.New("sitedex: too many redirects")

// FetchResult holds everything the crawler needs from one page fetch.
type FetchResult struct {
	StatusCode   int
	NotModified  bool // true on HTTP 304
	Body         []byte
	ContentType  string
	ETag         string
	LastModified string
	FinalURL     string // after following any redirects
}

// NewHTTPClient builds an http.Client configured for polite, bounded
// crawling: a capped number of redirects and no cookie jar (each request
// is independent).
func NewHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return ErrTooManyRedirects
			}
			return nil
		},
	}
}

// FetchPage fetches target with the given User-Agent, optionally sending
// conditional-GET headers when a prior ETag/Last-Modified are known (pass
// "" for either to omit it). A 304 response yields NotModified=true with
// no body.
func FetchPage(ctx context.Context, client *http.Client, userAgent, target, etag, lastModified string) (*FetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrTooManyRedirects) {
			return nil, ErrTooManyRedirects
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	result := &FetchResult{
		StatusCode:   resp.StatusCode,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FinalURL:     resp.Request.URL.String(),
	}

	if resp.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	result.Body = body
	return result, nil
}
