// Package ratelimit implements an in-memory token bucket per key (client IP).
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter allows `limit` requests per `window` per key, refilled
// continuously (token bucket with capacity `limit`).
type Limiter struct {
	mu      sync.Mutex
	limit   float64
	rate    float64 // tokens per second
	buckets map[string]*bucket
	now     func() time.Time
	lastGC  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a limiter for limit requests per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   float64(limit),
		rate:    float64(limit) / window.Seconds(),
		buckets: map[string]*bucket{},
		now:     time.Now,
	}
}

// SetClock replaces the clock (tests).
func (l *Limiter) SetClock(now func() time.Time) { l.now = now }

// Allow consumes a token for key. When none is left it returns false and
// the number of seconds until the next token (>= 1).
func (l *Limiter) Allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gc(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.limit, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(l.limit, b.tokens+elapsed*l.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	retry := int(math.Ceil((1 - b.tokens) / l.rate))
	if retry < 1 {
		retry = 1
	}
	return false, retry
}

// gc drops full buckets every few minutes so memory stays bounded.
func (l *Limiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < 5*time.Minute {
		return
	}
	l.lastGC = now
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.limit {
			delete(l.buckets, k)
		}
	}
}
