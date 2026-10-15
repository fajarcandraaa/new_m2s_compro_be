package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(5, time.Minute)
	l.SetClock(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d denied", i+1)
		}
	}
	ok, retry := l.Allow("a")
	if ok || retry < 1 || retry > 12 {
		t.Fatalf("6th: ok=%v retry=%d", ok, retry)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("keys must be independent")
	}
	now = now.Add(12 * time.Second) // one token refilled (5/min)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("token should refill")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("only one token refilled")
	}
	now = now.Add(10 * time.Minute)
	l.Allow("c") // triggers gc
	if len(l.buckets) != 1 {
		t.Fatalf("gc kept %d buckets", len(l.buckets))
	}
}
