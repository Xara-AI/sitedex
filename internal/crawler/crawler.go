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

// Logf is a minimal structured-ish logging hook; nil is fine (silent).
type Logf func(format string, args ...any)

// Crawler crawls one site: robots.txt + sitemap discovery, BFS traversal
// within scope, conditional revalidation, content extraction, and
// markdown export.
type Crawler struct {
	cfg     config.CrawlConfig
	dataDir string
	client  *http.Client
	writer  PageWriter
	log     Logf
}

// New builds a Crawler from the crawl section of the config and the
// data_dir it should write into.
func New(cfg config.CrawlConfig, dataDir string, writer PageWriter, log Logf) *Crawler {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Crawler{
		cfg:     cfg,
		dataDir: dataDir,
		client:  NewHTTPClient(),
		writer:  writer,
		log:     log,
	}
}

// Crawl crawls siteURL to completion (or until ctx is canceled), writing
// markdown pages via the configured PageWriter and persisting revalidation
// state to <data_dir>/<site>/crawl-state.json.
func (c *Crawler) Crawl(ctx context.Context, siteURL string) (*Result, error) {
	start := time.Now()

	seed, err := url.Parse(siteURL)
	if err != nil {
		return nil, fmt.Errorf("parse site URL: %w", err)
	}
	if seed.Scheme == "" {
		seed.Scheme = "https"
	}
	if seed.Host == "" {
		return nil, fmt.Errorf("site URL %q has no host", siteURL)
	}
	seed = NormalizeURL(seed)

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
