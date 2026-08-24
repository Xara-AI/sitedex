package crawler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

// maxSitemapDepth caps recursion through sitemap index files, so a
// misconfigured or malicious sitemap index chain can't loop forever.
const maxSitemapDepth = 5

// maxSitemapURLs caps the total number of URLs collected from sitemaps, as
// a defensive bound independent of crawl.max_pages (which is enforced
// separately by the frontier).
const maxSitemapURLs = 50000

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

type sitemapEntry struct {
	Loc string `xml:"loc"`
}

// FetchSitemapURLs fetches sitemapURL and returns every page URL it lists,
// recursing into nested sitemap index files (up to maxSitemapDepth). It is
// tolerant of partial failures: an unreachable or malformed sitemap simply
// contributes no URLs rather than failing the whole crawl.
func FetchSitemapURLs(ctx context.Context, client *http.Client, userAgent, sitemapURLStr string) []string {
	seen := make(map[string]bool)
	var out []string
	fetchSitemapRecursive(ctx, client, userAgent, sitemapURLStr, 0, seen, &out)
	return out
}

func fetchSitemapRecursive(ctx context.Context, client *http.Client, userAgent, loc string, depth int, seen map[string]bool, out *[]string) {
	if depth > maxSitemapDepth || len(*out) >= maxSitemapURLs || seen[loc] {
		return
	}
	seen[loc] = true

	body, err := fetchBody(ctx, client, userAgent, loc)
	if err != nil {
		return
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(io.LimitReader(body, 32*1024*1024))
	if err != nil {
		return
	}

	var set sitemapURLSet
	if err := xml.Unmarshal(data, &set); err == nil && len(set.URLs) > 0 {
		for _, u := range set.URLs {
			if u.Loc == "" {
				continue
			}
			*out = append(*out, u.Loc)
			if len(*out) >= maxSitemapURLs {
				return
			}
		}
		return
	}

	var idx sitemapIndex
	if err := xml.Unmarshal(data, &idx); err == nil && len(idx.Sitemaps) > 0 {
		for _, s := range idx.Sitemaps {
			if s.Loc == "" {
				continue
			}
			fetchSitemapRecursive(ctx, client, userAgent, s.Loc, depth+1, seen, out)
			if len(*out) >= maxSitemapURLs {
				return
			}
		}
	}
}

func fetchBody(ctx context.Context, client *http.Client, userAgent, target string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	// Deliberately not setting Accept-Encoding: the stdlib transport
	// auto-negotiates gzip and transparently decompresses it, but only
	// when the caller leaves this header unset.
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("sitemap fetch %s: status %d", target, resp.StatusCode)
	}
	return resp.Body, nil
}
