package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	pairRateLimitMax    = 10
	pairRateLimitWindow = time.Minute
	maxQueryLimit       = 1000
)

type ipRateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
}

func newIPRateLimiter(max int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		max:    max,
		window: window,
		now:    time.Now,
		hits:   make(map[string][]time.Time),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	if l == nil {
		return true
	}
	now := l.now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.hits) > 128 {
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}

	ts := l.hits[ip]
	keep := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= l.max {
		l.hits[ip] = keep
		return false
	}
	keep = append(keep, now)
	l.hits[ip] = keep
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
