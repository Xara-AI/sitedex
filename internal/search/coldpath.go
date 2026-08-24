package search

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/extract/product"
	"golang.org/x/net/html"
)

// coldPathTimeout bounds the whole cold-path fallback (homepage fetch +
// platform/form detection + search-results fetch + extraction). It's not
// part of the documented config schema — CLAUDE.md's "Hard response
// budget" section is specifically about fresh:true; cold path is already
// the slow-fallback case by nature, so this just keeps a truly
// unresponsive site from hanging a request indefinitely.
const coldPathTimeout = 5 * time.Second

const perColdFetchTimeout = 2 * time.Second

// siteBaseURL resolves a bare site key (e.g. "example.com") to the base
// URL cold-path detection starts from. Assumes https, since that's
// virtually universal for real e-commerce sites today. It's a package
// variable (rather than a hardcoded url.Parse call inline) specifically
// so tests can point it at a local httptest server's http:// URL.
var siteBaseURL = func(site string) (*url.URL, error) {
	return url.Parse("https://" + site + "/")
}

// coldSearch implements the cold path: the requested site has no index
// yet (or a caller explicitly wants a live site-search — see
// Request.ForceSiteSearch), so there's nothing to warm-search. Instead,
// detect the site's own on-site search endpoint, fetch it, and extract
// whatever products are on the results page — per CLAUDE.md's "Cold
// path". Any failure along the way (site unreachable, robots.txt
// disallows it, no recognizable search mechanism, nothing extractable)
// degrades to an empty result set with source "index" rather than an
// error, matching "empty results is a normal response". Like the
// crawler, cold path respects robots.txt for both pages it fetches — a
// site-search fallback shouldn't be less polite than a real crawl.
func (s *Searcher) coldSearch(ctx context.Context, req Request, limit int) Response {
	ctx, cancel := context.WithTimeout(ctx, coldPathTimeout)
	defer cancel()

	base, err := siteBaseURL(req.Site)
	if err != nil {
		return Response{Source: "index"}
	}

	client := crawler.NewHTTPClient()
	robots := fetchColdRobots(ctx, client, s.userAgent, base)

	if !robots.Allowed(base.Path) {
		return Response{Source: "index"}
	}
	home, err := fetchColdPage(ctx, client, s.userAgent, base.String())
	if err != nil {
		return Response{Source: "index"}
	}
	homeDoc, err := html.Parse(strings.NewReader(string(home)))
	if err != nil {
		return Response{Source: "index"}
	}

	searchURL, ok := buildSearchURL(homeDoc, base, req.Query)
	if !ok {
		return Response{Source: "index"}
	}
	searchURLParsed, err := url.Parse(searchURL)
	if err != nil {
		return Response{Source: "index"}
	}
	if !robots.Allowed(pathAndQuery(searchURLParsed)) {
		return Response{Source: "index"}
	}

	resultsPage, err := fetchColdPage(ctx, client, s.userAgent, searchURL)
	if err != nil {
		return Response{Source: "index"}
	}

	resultsURL, err := url.Parse(searchURL)
	if err != nil {
		resultsURL = base
	}
	products := product.ExtractList(resultsPage, resultsURL)
	if len(products) == 0 {
		return Response{Source: "index"}
	}
	if len(products) > limit {
		products = products[:limit]
	}

	results := make([]Result, len(products))
	for i, p := range products {
		results[i] = Result{
			URL: p.URL, Type: "product", Title: p.Name, Snippet: p.Description,
			Price: p.Price, HasPrice: p.HasPrice, Currency: p.Currency,
			Availability: string(p.Availability), Image: p.Image, ExtractionMethod: string(p.ExtractionMethod),
			// No BM25 available here (nothing is indexed yet); preserve the
			// results page's own ordering via a decreasing positional score.
			Score: positionalScore(i),
		}
	}
	return Response{Results: results, Source: "site-search"}
}

func positionalScore(i int) float64 {
	score := 1.0 - float64(i)*0.05
	if score < 0.1 {
		score = 0.1
	}
	return score
}

func fetchColdPage(ctx context.Context, client *http.Client, userAgent, target string) ([]byte, error) {
	fctx, cancel := context.WithTimeout(ctx, perColdFetchTimeout)
	defer cancel()
	res, err := crawler.FetchPage(fctx, client, userAgent, target, "", "")
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", res.StatusCode, target)
	}
	return res.Body, nil
}

// fetchColdRobots fetches and parses base's robots.txt, the same way the
// crawler does — an unreachable or missing robots.txt is treated as
// allow-all (see crawler.ParseRobots), not a failure.
func fetchColdRobots(ctx context.Context, client *http.Client, userAgent string, base *url.URL) *crawler.Robots {
	robotsURL := &url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/robots.txt"}
	fctx, cancel := context.WithTimeout(ctx, perColdFetchTimeout)
	defer cancel()
	res, err := crawler.FetchPage(fctx, client, userAgent, robotsURL.String(), "", "")
	if err != nil || res.StatusCode != http.StatusOK {
		return crawler.ParseRobots(strings.NewReader(""), userAgent)
	}
	return crawler.ParseRobots(bytes.NewReader(res.Body), userAgent)
}

// pathAndQuery returns u's path+query, which is what robots.txt patterns
// are matched against (a pattern like "/*?sort=" needs the query string,
// which u.Path alone does not include).
func pathAndQuery(u *url.URL) string {
	if u.RawQuery == "" {
		return u.Path
	}
	return u.Path + "?" + u.RawQuery
}

// buildSearchURL detects homeDoc's platform and returns that platform's
// known search-endpoint URL (trying documented alternates in order, per
// CLAUDE.md's "Cold path"), falling back to generic <form role="search">
// discovery when no platform is recognized.
func buildSearchURL(homeDoc *html.Node, base *url.URL, query string) (string, bool) {
	if candidates := platformSearchURLs(product.DetectPlatform(homeDoc), base, query); len(candidates) > 0 {
		return candidates[0], true
	}

	action, param, ok := findSearchForm(homeDoc)
	if !ok {
		return "", false
	}
	target := base.ResolveReference(&url.URL{Path: action})
	q := target.Query()
	q.Set(param, query)
	target.RawQuery = q.Encode()
	return target.String(), true
}

// platformSearchURLs returns platform's known site-search URL(s), in
// preference order, per CLAUDE.md: WooCommerce "/?s=", Shopify
// "/search?q=", PrestaShop "/recherche?s=" or
// "/search?controller=search&s=", OpenCart
// "index.php?route=product/search&search=". Only the first is currently
// used by buildSearchURL (trying alternates on a failed fetch is a
// reasonable future improvement, not implemented in v1).
func platformSearchURLs(p product.Platform, base *url.URL, query string) []string {
	switch p {
	case product.PlatformWooCommerce:
		return []string{withQuery(base, "/", url.Values{"s": {query}})}
	case product.PlatformShopify:
		return []string{withQuery(base, "/search", url.Values{"q": {query}})}
	case product.PlatformPrestaShop:
		return []string{
			withQuery(base, "/search", url.Values{"controller": {"search"}, "s": {query}}),
			withQuery(base, "/recherche", url.Values{"s": {query}}),
		}
	case product.PlatformOpenCart:
		return []string{withQuery(base, "/index.php", url.Values{"route": {"product/search"}, "search": {query}})}
	default:
		return nil
	}
}

func withQuery(base *url.URL, path string, values url.Values) string {
	u := *base
	u.Path = path
	u.RawQuery = values.Encode()
	u.Fragment = ""
	return u.String()
}

// findSearchForm looks for a <form role="search"> (falling back to
// class="search-form") with a GET method (or no explicit method — GET is
// the HTML default) and a text/search <input> with a name attribute,
// returning the form's action and that input's name.
func findSearchForm(doc *html.Node) (action, paramName string, ok bool) {
	form := findFormByRole(doc, "search")
	if form == nil {
		if c := findByClassTag(doc, "search-form", "form"); c != nil {
			form = c
		}
	}
	if form == nil {
		return "", "", false
	}
	if method := strings.ToLower(attrVal(form, "method")); method != "" && method != "get" {
		return "", "", false
	}
	input := findSearchInputName(form)
	if input == "" {
		return "", "", false
	}
	return attrVal(form, "action"), input, true
}

func findFormByRole(n *html.Node, role string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "form" && strings.EqualFold(attrVal(n, "role"), role) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFormByRole(c, role); found != nil {
			return found
		}
	}
	return nil
}

func findByClassTag(n *html.Node, class, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag && hasClass(n, class) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByClassTag(c, class, tag); found != nil {
			return found
		}
	}
	return nil
}

func findSearchInputName(form *html.Node) string {
	var result string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if result != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "input" {
			typ := strings.ToLower(attrVal(n, "type"))
			name := attrVal(n, "name")
			if name != "" && (typ == "search" || typ == "text" || typ == "") {
				result = name
				return
			}
		}
		for c := n.FirstChild; c != nil && result == ""; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
	return result
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attrVal(n, "class")) {
		if strings.EqualFold(c, class) {
			return true
		}
	}
	return false
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
