package mgogocron_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mgogocron "github.com/mgo-framework/mgo/adapters/sched-gocron"
	cachec "github.com/mgo-framework/mgo/contracts/cache"
)

// memLocker is a minimal in-process Locker so this module needs no
// framework dependency; production uses cache-redis behind the same
// contract.
type memLocker struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func newMemLocker() *memLocker { return &memLocker{until: map[string]time.Time{}} }

func (l *memLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().Before(l.until[key]) {
		return nil, false, nil
	}
	l.until[key] = time.Now().Add(ttl)
	return func(context.Context) error {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.until, key)
		return nil
	}, true, nil
}

var _ cachec.Locker = (*memLocker)(nil)

// start runs a scheduler until the test ends.
func start(t *testing.T, s *mgogocron.Scheduler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("scheduler did not stop")
		}
	})
}

// TestSoakLite: a 40ms job ticks steadily over ~600ms and stops cleanly.
func TestSoakLite(t *testing.T) {
	s, err := mgogocron.New()
	if err != nil {
		t.Fatal(err)
	}
	var ticks atomic.Int32
	if err := s.Every(40*time.Millisecond, "tick", func(ctx context.Context) error {
		ticks.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	start(t, s)
	time.Sleep(600 * time.Millisecond)
	if n := ticks.Load(); n < 5 {
		t.Fatalf("only %d ticks in 600ms at a 40ms interval", n)
	}
}

// TestWithoutOverlapping: a job slower than its interval never runs
// concurrently with itself.
func TestWithoutOverlapping(t *testing.T) {
	s, err := mgogocron.New()
	if err != nil {
		t.Fatal(err)
	}
	var inFlight, maxInFlight atomic.Int32
	if err := s.Every(30*time.Millisecond, "slow", func(ctx context.Context) error {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := maxInFlight.Load()
			if n <= old || maxInFlight.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		return nil
	}, mgogocron.WithoutOverlapping()); err != nil {
		t.Fatal(err)
	}
	start(t, s)
	time.Sleep(500 * time.Millisecond)
	if maxInFlight.Load() > 1 {
		t.Fatalf("max concurrent runs = %d, want 1", maxInFlight.Load())
	}
}

// TestOnOneServer: two schedulers sharing one locker — each tick runs on
// exactly one of them, so the total is far below double.
func TestOnOneServer(t *testing.T) {
	locker := newMemLocker()
	var total atomic.Int32
	job := func(ctx context.Context) error { total.Add(1); return nil }

	for range 2 {
		s, err := mgogocron.New()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Every(60*time.Millisecond, "shared", job,
			mgogocron.OnOneServer(locker, 55*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		start(t, s)
	}

	time.Sleep(650 * time.Millisecond)
	got := total.Load()
	// One server alone would tick ~10 times; two unguarded would be ~20.
	if got == 0 {
		t.Fatal("no ticks at all")
	}
	if got > 14 {
		t.Fatalf("total ticks = %d — both servers are running the job", got)
	}
}
