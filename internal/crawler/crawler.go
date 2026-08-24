package crawler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/extract/content"
	"github.com/Xara-AI/sitedex/internal/extract/product"
)

// Result summarizes one crawl run.
type Result struct {
	Site         string // registrable domain, and the <data_dir> subdirectory used
	PagesFetched int    // 200 responses that were processed
	PagesSkipped int    // 304 / unchanged-hash responses
	PagesFailed  int    // fetch or extraction errors
	PagesVisited int    // total URLs dequeued from the frontier
	Duration     time.Duration
}

// PageWriter persists one extracted page (e.g. as a markdown file). It's an
// interface so the crawler doesn't depend on the export package's concrete
// filesystem layout, and so tests can substitute an in-memory recorder.
type PageWriter interface {
	WritePage(kbDir string, pageURL *url.URL, page *content.Page) (relPath string, err error)
}

// Indexer persists one page's searchable content (metadata + chunks, and
// separately its product data if any) into the site's index. It's an
// interface for the same reason PageWriter is: the crawler doesn't need to
// know about SQLite, and tests can substitute an in-memory recorder. nil
// is valid — indexing is then simply skipped (e.g. for callers that only
// want the markdown export).
type Indexer interface {
	IndexPage(page PageForIndex, chunks []ChunkForIndex) error
	// IndexProduct upserts (p != nil) or removes (p == nil) the product
	// record for pageURL. Most pages aren't products, so p is nil far more
	// often than not — that's a normal call, not a special case to avoid.
	IndexProduct(pageURL string, p *ProductForIndex) error
}

// PageForIndex, ChunkForIndex, and ProductForIndex mirror internal/index's
// PageRecord/ChunkRecord/ProductRecord shapes without the crawler package
// depending on internal/index directly (same decoupling reasoning as
// PageWriter).
type PageForIndex struct {
	URL          string
	Title        string
	Description  string
	Lang         string
	Hash         string
	CrawledAt    time.Time
	ETag         string
	LastModified string
}

type ChunkForIndex struct {
	Ordinal     int
	HeadingPath string
	Text        string
}

type ProductForIndex struct {
	Name             string
	Description      string
	Price            float64
	HasPrice         bool
	Currency         string
	Availability     string
	Image            string
	ExtractionMethod string
	RawJSON          string
}

// Logf is a minimal structured-ish logging hook; nil is fine (silent).
type Logf func(format string, args ...any)

// Crawler crawls one site: robots.txt + sitemap discovery, BFS traversal
// within scope, conditional revalidation, content extraction, markdown
// export, and search indexing.
type Crawler struct {
	cfg      config.CrawlConfig
	chunking config.ChunkingConfig
	dataDir  string
	client   *http.Client
	writer   PageWriter
	indexer  Indexer
	log      Logf
}

// New builds a Crawler from the crawl/chunking sections of the config and
// the data_dir it should write into. indexer may be nil to skip indexing
// (markdown-only crawl).
func New(cfg config.CrawlConfig, chunking config.ChunkingConfig, dataDir string, writer PageWriter, indexer Indexer, log Logf) *Crawler {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Crawler{
		cfg:      cfg,
		chunking: chunking,
		dataDir:  dataDir,
		client:   NewHTTPClient(),
		writer:   writer,
		indexer:  indexer,
		log:      log,
	}
}

// parseSeedURL parses and normalizes a user-supplied site URL, defaulting
// to https:// when no scheme is given.
func parseSeedURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse site URL: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Host == "" {
		return nil, fmt.Errorf("site URL %q has no host", rawURL)
	}
	return NormalizeURL(u), nil
}

// SiteForURL returns the registrable-domain identifier sitedex uses as a
// site's data-dir key, e.g. "https://shop.example.com/x" -> "example.com".
// Callers that need to open a site's index before/independent of a crawl
// (e.g. the CLI) use this to resolve the same key Crawl uses internally.
func SiteForURL(rawURL string) (string, error) {
	u, err := parseSeedURL(rawURL)
	if err != nil {
		return "", err
	}
	return RegistrableDomain(u.Host), nil
}

// Crawl crawls siteURL to completion (or until ctx is canceled), writing
// markdown pages via the configured PageWriter, indexing them via the
// configured Indexer, and persisting revalidation state to
// <data_dir>/<site>/crawl-state.json.
func (c *Crawler) Crawl(ctx context.Context, siteURL string) (*Result, error) {
	start := time.Now()

	seed, err := parseSeedURL(siteURL)
	if err != nil {
		return nil, err
	}

	site := RegistrableDomain(seed.Host)
	kbDir := c.dataDir + "/" + site + "/kb"

	state, err := OpenStateStore(c.dataDir, site)
	if err != nil {
		return nil, fmt.Errorf("open crawl state: %w", err)
	}

	robots := c.fetchRobots(ctx, seed)

	limiter := NewHostRateLimiter(rateLimitInterval(c.cfg.RateLimitRPS))
	if d := robots.CrawlDelay(); d > 0 {
		limiter.SetHostInterval(seed.Host, d)
	}

	frontier := NewFrontier(seed, c.cfg.MaxDepth, c.cfg.Include, c.cfg.Exclude)
	frontier.Add(seed, 0)

	for _, loc := range c.discoverSitemapURLs(ctx, seed, robots) {
		if u, err := url.Parse(loc); err == nil && u.Host != "" {
			frontier.Add(NormalizeURL(u), 1)
		}
	}

	result := &Result{Site: site}

	for result.PagesVisited < c.cfg.MaxPages {
		u, depth, ok := frontier.Next()
		if !ok {
			break
		}
		result.PagesVisited++

		if err := ctx.Err(); err != nil {
			c.log("crawl: context canceled, stopping (%d pages visited)", result.PagesVisited)
			break
		}

		if !robots.Allowed(robotsPath(u)) {
			c.log("crawl: robots disallow %s", u.String())
			continue
		}

		if err := limiter.Wait(ctx, u.Host); err != nil {
			break // context canceled while waiting
		}

		links, changed, err := c.fetchAndProcess(ctx, u, state, kbDir)
		if err != nil {
			result.PagesFailed++
			c.log("crawl: fetch %s: %v", u.String(), err)
			continue
		}
		if !changed {
			result.PagesSkipped++
		} else {
			result.PagesFetched++
		}

		if depth < c.cfg.MaxDepth {
			for _, l := range links {
				frontier.Add(l, depth+1)
			}
		}
	}

	if err := state.Save(); err != nil {
		return result, fmt.Errorf("save crawl state: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// fetchAndProcess fetches one URL (conditionally, if we have prior state),
// and on a genuine 200 with changed content, extracts + writes it and
// returns the links found on the page. It returns changed=false for 304s
// and for 200s whose content hash didn't move (both count as "skipped" by
// the caller).
func (c *Crawler) fetchAndProcess(ctx context.Context, u *url.URL, state *StateStore, kbDir string) (links []*url.URL, changed bool, err error) {
	key := u.String()
	prior, hasPrior := state.Get(key)

	etag, lastMod := "", ""
	if hasPrior {
		etag, lastMod = prior.ETag, prior.LastModified
	}

	res, err := FetchPage(ctx, c.client, c.cfg.UserAgent, key, etag, lastMod)
	if err != nil {
		return nil, false, err
	}

	if res.NotModified {
		prior.CrawledAt = time.Now().UTC()
		state.Set(key, prior)
		return nil, false, nil
	}

	if res.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("status %d", res.StatusCode)
	}

	links = ExtractLinks(u, res.Body)

	if !isHTML(res.ContentType) {
		// Not a page we can extract content from (e.g. a PDF a sitemap
		// pointed at); still worth having discovered its outbound links
		// only if it was actually HTML, so skip both extraction and link
		// discovery for non-HTML responses.
		return nil, false, nil
	}

	page, err := content.Extract(res.Body, u, time.Now().UTC())
	if err != nil {
		return nil, false, fmt.Errorf("extract content: %w", err)
	}

	if hasPrior && prior.Hash == page.Hash {
		// Content unchanged: refresh revalidation metadata but skip the
		// write, per CLAUDE.md's "unchanged hash = no write" guidance.
		state.Set(key, PageState{ETag: res.ETag, LastModified: res.LastModified, Hash: page.Hash, CrawledAt: time.Now().UTC()})
		return links, false, nil
	}

	if _, err := c.writer.WritePage(kbDir, u, page); err != nil {
		return nil, false, fmt.Errorf("write page: %w", err)
	}

	if c.indexer != nil {
		chunks := content.ChunkPage(page, c.chunking.TargetChars, c.chunking.OverlapChars)
		idxChunks := make([]ChunkForIndex, len(chunks))
		for i, ch := range chunks {
			idxChunks[i] = ChunkForIndex{Ordinal: ch.Ordinal, HeadingPath: ch.HeadingPath, Text: ch.Text}
		}
		idxPage := PageForIndex{
			URL: page.URL, Title: page.Title, Description: page.Description, Lang: page.Lang,
			Hash: page.Hash, CrawledAt: page.CrawledAt, ETag: res.ETag, LastModified: res.LastModified,
		}
		if err := c.indexer.IndexPage(idxPage, idxChunks); err != nil {
			return nil, false, fmt.Errorf("index page: %w", err)
		}

		var idxProduct *ProductForIndex
		if prod, ok := product.Extract(res.Body, u); ok {
			idxProduct = &ProductForIndex{
				Name: prod.Name, Description: prod.Description, Price: prod.Price, HasPrice: prod.HasPrice,
				Currency: prod.Currency, Availability: string(prod.Availability), Image: prod.Image,
				ExtractionMethod: string(prod.ExtractionMethod), RawJSON: prod.RawJSON,
			}
		}
		if err := c.indexer.IndexProduct(page.URL, idxProduct); err != nil {
			return nil, false, fmt.Errorf("index product: %w", err)
		}
	}

	state.Set(key, PageState{ETag: res.ETag, LastModified: res.LastModified, Hash: page.Hash, CrawledAt: time.Now().UTC()})

	return links, true, nil
}

func isHTML(contentType string) bool {
	if contentType == "" {
		return true // be permissive: many misconfigured servers omit/mis-set this
	}
	for _, prefix := range []string{"text/html", "application/xhtml+xml"} {
		if len(contentType) >= len(prefix) && contentType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func (c *Crawler) fetchRobots(ctx context.Context, seed *url.URL) *Robots {
	if !c.cfg.RespectRobots {
		// Permissive empty ruleset. Overriding respect_robots is documented
		// as being for sites you own or have permission to crawl.
		return ParseRobots(strings.NewReader(""), c.cfg.UserAgent)
	}
	robotsURL := &url.URL{Scheme: seed.Scheme, Host: seed.Host, Path: "/robots.txt"}
	res, err := FetchPage(ctx, c.client, c.cfg.UserAgent, robotsURL.String(), "", "")
	if err != nil || res.StatusCode != http.StatusOK {
		c.log("crawl: no usable robots.txt at %s (%v)", robotsURL, err)
		return ParseRobots(strings.NewReader(""), c.cfg.UserAgent)
	}
	return ParseRobots(bytes.NewReader(res.Body), c.cfg.UserAgent)
}

func (c *Crawler) discoverSitemapURLs(ctx context.Context, seed *url.URL, robots *Robots) []string {
	var sitemaps []string
	sitemaps = append(sitemaps, robots.Sitemaps()...)
	if len(sitemaps) == 0 {
		def := &url.URL{Scheme: seed.Scheme, Host: seed.Host, Path: "/sitemap.xml"}
		sitemaps = append(sitemaps, def.String())
	}

	var urls []string
	for _, sm := range sitemaps {
		urls = append(urls, FetchSitemapURLs(ctx, c.client, c.cfg.UserAgent, sm)...)
	}
	return urls
}

// robotsPath returns the path+query that robots.txt rules are matched
// against, per the robots.txt spec (patterns like "/*?sort=" need the query
// string, which url.URL.Path alone does not include).
func robotsPath(u *url.URL) string {
	if u.RawQuery == "" {
		return u.Path
	}
	return u.Path + "?" + u.RawQuery
}

func rateLimitInterval(rps float64) time.Duration {
	if rps <= 0 {
		rps = 1.0
	}
	return time.Duration(float64(time.Second) / rps)
}
