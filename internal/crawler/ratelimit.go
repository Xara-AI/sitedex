package crawler

import (
	"context"
	"sync"
	"time"
)

// HostRateLimiter enforces a minimum interval between requests to each
// host, independently per host, so a crawl of multiple hosts (e.g.
// subdomains under the same registrable domain) doesn't slow down more
// than necessary while still being polite to each one individually.
type HostRateLimiter struct {
	mu       sync.Mutex
	interval map[string]time.Duration // per-host override (e.g. robots Crawl-delay), else default
	next     map[string]time.Time     // next time a request to this host may fire
	def      time.Duration
}

// NewHostRateLimiter creates a limiter with the given default interval
// between requests to any one host (derived from rate_limit_rps).
func NewHostRateLimiter(defaultInterval time.Duration) *HostRateLimiter {
	return &HostRateLimiter{
		interval: make(map[string]time.Duration),
		next:     make(map[string]time.Time),
		def:      defaultInterval,
	}
}

// SetHostInterval overrides the minimum interval for a specific host (used
// when robots.txt specifies a Crawl-delay longer than the configured
// default).
func (l *HostRateLimiter) SetHostInterval(host string, interval time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.interval[host] = interval
}

// Wait blocks until it is this host's turn to fire a request, or ctx is
// canceled. It always reserves the next slot before returning nil, so
// concurrent callers for the same host are naturally serialized at the
// configured rate.
func (l *HostRateLimiter) Wait(ctx context.Context, host string) error {
	for {
		l.mu.Lock()
		interval := l.interval[host]
		if interval <= 0 {
			interval = l.def
		}
		now := time.Now()
		next, ok := l.next[host]
		if !ok || !next.After(now) {
			l.next[host] = now.Add(interval)
			l.mu.Unlock()
			return nil
		}
		wait := next.Sub(now)
		l.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// loop back around and re-check/reserve, in case another
			// goroutine grabbed the slot while we were waiting.
		}
	}
}
