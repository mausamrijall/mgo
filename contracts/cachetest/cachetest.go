// Package cachetest is the conformance suite for contracts/cache
// implementations. Drivers run Run (and RunLocker/RunCounter for the
// capabilities they implement) in their tests.
package cachetest

import (
	"bytes"
	"context"
	"testing"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
)

// Run exercises the base Store contract. newStore must return a fresh,
// empty store per call.
func Run(t *testing.T, newStore func() cachec.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("set get roundtrip", func(t *testing.T) {
		s := newStore()
		if err := s.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
			t.Fatal(err)
		}
		v, ok, err := s.Get(ctx, "k")
		if err != nil || !ok || !bytes.Equal(v, []byte("v")) {
			t.Fatalf("get = %q %v %v", v, ok, err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		s := newStore()
		if _, ok, err := s.Get(ctx, "nope"); ok || err != nil {
			t.Fatalf("missing = %v %v, want false nil", ok, err)
		}
	})

	t.Run("ttl expiry", func(t *testing.T) {
		s := newStore()
		if err := s.Set(ctx, "k", []byte("v"), 30*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		time.Sleep(60 * time.Millisecond)
		if _, ok, _ := s.Get(ctx, "k"); ok {
			t.Fatal("expired key still present")
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		s := newStore()
		s.Set(ctx, "k", []byte("a"), time.Minute)
		s.Set(ctx, "k", []byte("b"), time.Minute)
		v, _, _ := s.Get(ctx, "k")
		if !bytes.Equal(v, []byte("b")) {
			t.Fatalf("overwrite = %q", v)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newStore()
		s.Set(ctx, "k", []byte("v"), time.Minute)
		if err := s.Delete(ctx, "k"); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := s.Get(ctx, "k"); ok {
			t.Fatal("deleted key still present")
		}
		if err := s.Delete(ctx, "absent"); err != nil {
			t.Fatalf("deleting absent key errored: %v", err)
		}
	})
}

// RunLocker exercises the Locker capability.
func RunLocker(t *testing.T, newLocker func() cachec.Locker) {
	t.Helper()
	ctx := context.Background()

	t.Run("exclusive while held", func(t *testing.T) {
		l := newLocker()
		release, ok, err := l.TryLock(ctx, "job", time.Minute)
		if err != nil || !ok {
			t.Fatalf("first lock = %v %v", ok, err)
		}
		if _, ok2, _ := l.TryLock(ctx, "job", time.Minute); ok2 {
			t.Fatal("second lock acquired while held")
		}
		if err := release(ctx); err != nil {
			t.Fatal(err)
		}
		if _, ok3, _ := l.TryLock(ctx, "job", time.Minute); !ok3 {
			t.Fatal("lock not reacquirable after release")
		}
	})

	t.Run("expires", func(t *testing.T) {
		l := newLocker()
		if _, ok, _ := l.TryLock(ctx, "short", 30*time.Millisecond); !ok {
			t.Fatal("lock failed")
		}
		time.Sleep(60 * time.Millisecond)
		if _, ok, _ := l.TryLock(ctx, "short", time.Minute); !ok {
			t.Fatal("expired lock still held")
		}
	})
}

// RunCounter exercises the Counter capability.
func RunCounter(t *testing.T, newCounter func() cachec.Counter) {
	t.Helper()
	ctx := context.Background()

	t.Run("increments", func(t *testing.T) {
		c := newCounter()
		for want := int64(1); want <= 3; want++ {
			got, err := c.Increment(ctx, "hits", time.Minute)
			if err != nil || got != want {
				t.Fatalf("increment = %d %v, want %d", got, err, want)
			}
		}
	})

	t.Run("window resets", func(t *testing.T) {
		c := newCounter()
		c.Increment(ctx, "w", 30*time.Millisecond)
		time.Sleep(60 * time.Millisecond)
		if got, _ := c.Increment(ctx, "w", time.Minute); got != 1 {
			t.Fatalf("after window = %d, want 1", got)
		}
	})
}
