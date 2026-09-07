// Package ratelimit provides a bounded, single-process token bucket.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens  float64
	updated time.Time
}
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]bucket
	capacity int
	period   time.Duration
	maxKeys  int
}

func New(capacity int, period time.Duration, maxKeys int) *Limiter {
	return &Limiter{buckets: make(map[string]bucket), capacity: capacity, period: period, maxKeys: maxKeys}
}
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= l.maxKeys {
			for k, v := range l.buckets {
				if now.Sub(v.updated) >= l.period {
					delete(l.buckets, k)
				}
			}
			if len(l.buckets) >= l.maxKeys {
				return false
			}
		}
		b = bucket{tokens: float64(l.capacity), updated: now}
	}
	b.tokens = min(float64(l.capacity), b.tokens+max(0, now.Sub(b.updated).Seconds())*float64(l.capacity)/l.period.Seconds())
	b.updated = now
	allowed := b.tokens >= 1
	if allowed {
		b.tokens--
	}
	l.buckets[key] = b
	return allowed
}
