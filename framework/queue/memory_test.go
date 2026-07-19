package queue_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	queuec "github.com/mgo-framework/mgo/contracts/queue"
	"github.com/mgo-framework/mgo/contracts/queuetest"
	"github.com/mgo-framework/mgo/framework/queue"
)

func TestConformance(t *testing.T) {
	queuetest.Run(t, func(t *testing.T) queuetest.Pair {
		m := queue.NewMemory(4, 5)
		return queuetest.Pair{Enqueuer: m, Worker: m}
	})
}

func TestDropsAfterMaxRetries(t *testing.T) {
	m := queue.NewMemory(2, 2)
	var attempts atomic.Int32
	m.Register("doomed", func(ctx context.Context, job queuec.Job) error {
		attempts.Add(1)
		return errors.New("always fails")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	m.Enqueue(ctx, queuec.Job{Type: "doomed"})
	time.Sleep(300 * time.Millisecond)
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want exactly 2 (maxRetry)", got)
	}
}

func TestPanicIsRetriedNotFatal(t *testing.T) {
	m := queue.NewMemory(1, 3)
	var attempts atomic.Int32
	m.Register("panicky", func(ctx context.Context, job queuec.Job) error {
		if attempts.Add(1) == 1 {
			panic("first attempt explodes")
		}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	m.Enqueue(ctx, queuec.Job{Type: "panicky"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if attempts.Load() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("panicking job not redelivered (attempts=%d)", attempts.Load())
}

// TestGracefulShutdownFinishesInFlight: cancel arrives mid-job; the
// worker completes the job before Run returns — queue:work integrated
// with app shutdown, the Phase 7 exit behavior.
func TestGracefulShutdownFinishesInFlight(t *testing.T) {
	m := queue.NewMemory(1, 1)
	started := make(chan struct{})
	var finished atomic.Bool
	m.Register("slow", func(ctx context.Context, job queuec.Job) error {
		close(started)
		time.Sleep(150 * time.Millisecond)
		finished.Store(true)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx) }()

	m.Enqueue(ctx, queuec.Job{Type: "slow"})
	<-started
	cancel() // shutdown lands while the job is in flight

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if !finished.Load() {
		t.Fatal("in-flight job was abandoned on shutdown")
	}
}
