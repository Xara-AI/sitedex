package crawler

import (
	"net/url"
	"regexp"
)

// frontierItem is one URL queued for crawling, at a known BFS depth from
// the seed.
type frontierItem struct {
	url   *url.URL
	depth int
}

// Frontier is a BFS queue of URLs to crawl, scoped to a single
// registrable domain, deduplicated, and filtered by include/exclude
// patterns and max depth.
type Frontier struct {
	registrableDomain string
	maxDepth          int
	include           []*regexp.Regexp
	exclude           []*regexp.Regexp

	queue   []frontierItem
	visited map[string]bool
}

// NewFrontier creates a Frontier scoped to seed's registrable domain.
// include/exclude are regex source strings (as in config.CrawlConfig); an
// invalid pattern is skipped rather than failing the crawl, since a typo
// in one exclude rule shouldn't block an entire run.
func NewFrontier(seed *url.URL, maxDepth int, include, exclude []string) *Frontier {
	f := &Frontier{
		registrableDomain: RegistrableDomain(seed.Host),
		maxDepth:          maxDepth,
		visited:           make(map[string]bool),
	}
	f.include = compilePatterns(include)
	f.exclude = compilePatterns(exclude)
	return f
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

// InScope reports whether u should be crawled at all: same registrable
// domain, and passing the include/exclude filters. It does not check
// robots.txt or depth — callers combine InScope with those separately
// (robots needs a live Robots value, and depth is enforced at Add time).
func (f *Frontier) InScope(u *url.URL) bool {
	if !SameRegistrableDomain(u.Host, f.registrableDomain) {
		return false
	}
	s := u.String()
	if len(f.include) > 0 {
		matched := false
		for _, re := range f.include {
			if re.MatchString(s) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, re := range f.exclude {
		if re.MatchString(s) {
			return false
		}
	}
	return true
}

// Add enqueues u at the given depth if it is in scope, within max depth,
// and not already seen (by any depth). Returns true if it was added.
func (f *Frontier) Add(u *url.URL, depth int) bool {
	if depth > f.maxDepth {
		return false
	}
	if !f.InScope(u) {
		return false
	}
	key := u.String()
	if f.visited[key] {
		return false
	}
	f.visited[key] = true
	f.queue = append(f.queue, frontierItem{url: u, depth: depth})
	return true
}

// Next pops the next item in BFS order. ok is false when the frontier is
// empty.
func (f *Frontier) Next() (u *url.URL, depth int, ok bool) {
	if len(f.queue) == 0 {
		return nil, 0, false
	}
	item := f.queue[0]
	f.queue = f.queue[1:]
	return item.url, item.depth, true
}

// Len reports how many URLs are currently queued.
func (f *Frontier) Len() int {
	return len(f.queue)
}

// Visited reports how many distinct URLs have ever been added (queued or
// popped), i.e. the dedup set size.
func (f *Frontier) Visited() int {
	return len(f.visited)
}
