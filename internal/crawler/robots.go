package crawler

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Robots holds the robots.txt rules that apply to a single user-agent
// (the most specific matching group, falling back to "*"), plus any
// Sitemap: directives, which apply regardless of group.
type Robots struct {
	rules      []robotsRule
	crawlDelay time.Duration
	sitemaps   []string
}

type robotsRule struct {
	re     *regexp.Regexp
	allow  bool
	length int // length of the original pattern, for "longest rule wins"
}

// ParseRobots parses a robots.txt document, keeping only the directives
// that apply to userAgent. It never errors: a missing, empty, or malformed
// robots.txt yields a Robots that allows everything, which is the safe
// default per the robots.txt spec.
func ParseRobots(r io.Reader, userAgent string) *Robots {
	ua := strings.ToLower(userAgent)

	type group struct {
		agents     []string
		rules      []robotsRule
		crawlDelay time.Duration
	}
	var groups []group
	var sitemaps []string
	var cur *group
	sawRuleSinceUA := true // forces a new group on the first User-agent line

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])

		switch key {
		case "user-agent":
			if cur == nil || sawRuleSinceUA {
				groups = append(groups, group{})
				cur = &groups[len(groups)-1]
				sawRuleSinceUA = false
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
		case "disallow":
			if cur != nil && val != "" {
				cur.rules = append(cur.rules, robotsRule{re: compileRobotsPattern(val), allow: false, length: len(val)})
				sawRuleSinceUA = true
			} else if cur != nil {
				sawRuleSinceUA = true
			}
		case "allow":
			if cur != nil && val != "" {
				cur.rules = append(cur.rules, robotsRule{re: compileRobotsPattern(val), allow: true, length: len(val)})
				sawRuleSinceUA = true
			} else if cur != nil {
				sawRuleSinceUA = true
			}
		case "crawl-delay":
			if cur != nil {
				if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0 {
					cur.crawlDelay = time.Duration(f * float64(time.Second))
				}
				sawRuleSinceUA = true
			}
		case "sitemap":
			if val != "" {
				sitemaps = append(sitemaps, val)
			}
		}
	}

	var wildcard, best *group
	bestLen := -1
	for i := range groups {
		g := &groups[i]
		for _, a := range g.agents {
			if a == "*" {
				if wildcard == nil {
					wildcard = g
				}
				continue
			}
			if a != "" && strings.Contains(ua, a) && len(a) > bestLen {
				best = g
				bestLen = len(a)
			}
		}
	}
	if best == nil {
		best = wildcard
	}

	rt := &Robots{sitemaps: sitemaps}
	if best != nil {
		rt.rules = best.rules
		rt.crawlDelay = best.crawlDelay
	}
	return rt
}

// compileRobotsPattern turns a robots.txt path pattern ("*" wildcard, "$"
// end-anchor) into a regular expression anchored at the start of the path.
func compileRobotsPattern(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')
	endAnchor := strings.HasSuffix(pattern, "$")
	p := strings.TrimSuffix(pattern, "$")
	for _, r := range p {
		if r == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if endAnchor {
		b.WriteByte('$')
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		// Escaped-literal patterns essentially never fail to compile; if
		// one somehow does, treat it as never-matching rather than
		// panicking or over-blocking.
		return regexp.MustCompile(`\x00NEVER-MATCH\x00`)
	}
	return re
}

// Allowed reports whether path may be fetched under these rules. Per the
// robots.txt spec, the longest matching pattern wins; a tie between an
// Allow and a Disallow of equal length favors Allow.
func (r *Robots) Allowed(path string) bool {
	if r == nil {
		return true
	}
	allowed := true
	bestLen := -1
	for _, rl := range r.rules {
		if !rl.re.MatchString(path) {
			continue
		}
		if rl.length > bestLen || (rl.length == bestLen && rl.allow) {
			bestLen = rl.length
			allowed = rl.allow
		}
	}
	return allowed
}

// CrawlDelay returns the Crawl-delay directive for the matched group, or 0
// if none was specified.
func (r *Robots) CrawlDelay() time.Duration {
	if r == nil {
		return 0
	}
	return r.crawlDelay
}

// Sitemaps returns the Sitemap: URLs advertised in the document, if any.
func (r *Robots) Sitemaps() []string {
	if r == nil {
		return nil
	}
	return r.sitemaps
}
