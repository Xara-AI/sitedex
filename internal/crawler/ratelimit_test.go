package crawler

import (
	"context"
	"testing"
	"time"
)

func TestHostRateLimiter_EnforcesInterval(t *testing.T) {
	l := NewHostRateLimiter(50 * time.Millisecond)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(ctx, "example.com"); err != nil {
			t.Fatalf("Wait: %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 90*time.Millisecond { // 2 intervals, minus a little slack
		t.Errorf("elapsed = %v, want at least ~100ms for 3 requests at 50ms interval", elapsed)
	}
}

func TestHostRateLimiter_IndependentPerHost(t *testing.T) {
	l := NewHostRateLimiter(200 * time.Millisecond)
	ctx := context.Background()

	start := time.Now()
	if err := l.Wait(ctx, "a.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := l.Wait(ctx, "b.example.com"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Errorf("elapsed = %v, want near-instant since hosts are independent", elapsed)
	}
}

func TestHostRateLimiter_HostOverride(t *testing.T) {
	l := NewHostRateLimiter(10 * time.Millisecond)
	l.SetHostInterval("slow.example.com", 100*time.Millisecond)
	ctx := context.Background()

	start := time.Now()
	if err := l.Wait(ctx, "slow.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := l.Wait(ctx, "slow.example.com"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 80*time.Millisecond {
		t.Errorf("elapsed = %v, want at least ~100ms honoring host override", elapsed)
	}
}

func TestHostRateLimiter_ContextCancel(t *testing.T) {
	l := NewHostRateLimiter(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	if err := l.Wait(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if err := l.Wait(ctx, "example.com"); err == nil {
		t.Error("expected Wait to return an error when context is canceled")
	}
}
