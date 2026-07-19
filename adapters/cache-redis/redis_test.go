package mgoredis_test

// Conformance against miniredis — a real Redis protocol implementation
// in-process, so the suite runs everywhere. miniredis needs manual clock
// advancement for TTLs, so expiry subtests get a ticker goroutine.

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	mgoredis "github.com/mgo-framework/mgo/adapters/cache-redis"
	cachec "github.com/mgo-framework/mgo/contracts/cache"
	"github.com/mgo-framework/mgo/contracts/cachetest"
	"github.com/redis/go-redis/v9"
)

// newStore boots a fresh miniredis and advances its clock in real time
// so the conformance suite's sleep-based TTL tests behave.
func newStore(t *testing.T) *mgoredis.Store {
	t.Helper()
	mr := miniredis.RunT(t)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				mr.FastForward(10 * time.Millisecond)
			}
		}
	}()
	return mgoredis.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
}

func TestConformance(t *testing.T) {
	cachetest.Run(t, func() cachec.Store { return newStore(t) })
	cachetest.RunLocker(t, func() cachec.Locker { return newStore(t) })
	cachetest.RunCounter(t, func() cachec.Counter { return newStore(t) })
}

func TestReleaseIsCompareAndDelete(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	release1, ok, err := s.TryLock(ctx, "job", 30*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	// Let it expire, let someone else take it.
	time.Sleep(80 * time.Millisecond)
	_, ok, err = s.TryLock(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("relock after expiry: %v %v", ok, err)
	}
	// The stale holder's release must NOT free the new holder's lock.
	if err := release1(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.TryLock(ctx, "job", time.Minute); ok {
		t.Fatal("stale release deleted the new holder's lock")
	}
}
