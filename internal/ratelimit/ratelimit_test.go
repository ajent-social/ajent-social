package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestRefillIsolationAndBound(t *testing.T) {
	l := New(2, time.Minute, 2)
	now := time.Unix(0, 0)
	if !l.Allow("a", now) || !l.Allow("a", now) || l.Allow("a", now) {
		t.Fatal("burst not enforced")
	}
	if !l.Allow("b", now) {
		t.Fatal("identities not isolated")
	}
	if l.Allow("c", now) {
		t.Fatal("key bound not enforced")
	}
	if !l.Allow("a", now.Add(30*time.Second)) {
		t.Fatal("bucket did not refill")
	}
	if !l.Allow("c", now.Add(2*time.Minute)) {
		t.Fatal("idle keys not reclaimed")
	}
}
func TestConcurrentLimit(t *testing.T) {
	l := New(10, time.Minute, 100)
	now := time.Now()
	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- l.Allow("one", now) }()
	}
	wg.Wait()
	close(results)
	n := 0
	for allowed := range results {
		if allowed {
			n++
		}
	}
	if n != 10 {
		t.Fatalf("allowed %d, want 10", n)
	}
}
