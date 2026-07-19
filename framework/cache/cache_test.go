package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
	"github.com/mgo-framework/mgo/contracts/cachetest"
	"github.com/mgo-framework/mgo/framework/cache"
)

func TestMemoryConformance(t *testing.T) {
	cachetest.Run(t, func() cachec.Store { return cache.NewMemory() })
	cachetest.RunLocker(t, func() cachec.Locker { return cache.NewMemory() })
	cachetest.RunCounter(t, func() cachec.Counter { return cache.NewMemory() })
}

func TestRememberCachesAndForgets(t *testing.T) {
	ctx := context.Background()
	s := cache.NewMemory()
	calls := 0
	fn := func(ctx context.Context) (string, error) { calls++; return "value", nil }

	for range 3 {
		v, err := cache.Remember(ctx, s, "k", time.Minute, fn)
		if err != nil || v != "value" {
			t.Fatalf("remember = %q %v", v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("upstream called %d times, want 1", calls)
	}

	cache.Forget(ctx, s, "k")
	cache.Remember(ctx, s, "k", time.Minute, fn)
	if calls != 2 {
		t.Fatalf("after Forget upstream called %d times, want 2", calls)
	}
}

// TestHerd is the Phase 7 exit gate: 1000 concurrent Remember calls for
// a cold key produce exactly ONE upstream call.
func TestHerd(t *testing.T) {
	ctx := context.Background()
	s := cache.NewMemory()
	var upstream atomic.Int32
	fn := func(ctx context.Context) (int, error) {
		upstream.Add(1)
		time.Sleep(50 * time.Millisecond) // slow upstream widens the race window
		return 42, nil
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 1000)
	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := cache.Remember(ctx, s, "hot", time.Minute, fn)
			if err != nil || v != 42 {
				select {
				case errs <- err:
				default:
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatalf("remember failed: %v", err)
	default:
	}
	if n := upstream.Load(); n != 1 {
		t.Fatalf("upstream called %d times for 1000 concurrent callers, want 1", n)
	}
}

func TestRememberDistinctKeysDontCollapse(t *testing.T) {
	ctx := context.Background()
	s := cache.NewMemory()
	var calls atomic.Int32
	var wg sync.WaitGroup
	for _, key := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Remember(ctx, s, key, time.Minute, func(ctx context.Context) (string, error) {
				calls.Add(1)
				return key, nil
			})
		}()
	}
	wg.Wait()
	if calls.Load() != 3 {
		t.Fatalf("distinct keys collapsed: %d calls, want 3", calls.Load())
	}
}
