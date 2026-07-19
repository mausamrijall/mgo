// Package queuetest is the conformance suite for contracts/queue
// drivers: delivery, redelivery-on-failure, and delayed jobs.
package queuetest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	queuec "github.com/mgo-framework/mgo/contracts/queue"
)

// Pair is one driver instance under test. Enqueuer and Worker may be the
// same object (memory driver) or two ends of a broker (asynq).
type Pair struct {
	Enqueuer queuec.Enqueuer
	Worker   queuec.Worker
}

// Run exercises the queue contract. setup must return a fresh driver per
// call; the suite runs each Worker until the subtest ends.
func Run(t *testing.T, setup func(t *testing.T) Pair) {
	t.Helper()

	start := func(t *testing.T, w queuec.Worker) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); w.Run(ctx) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("worker did not stop after cancel")
			}
		})
	}

	// Generous deadline: broker drivers may poll (asynq forwards
	// scheduled tasks every ~5s); fast paths return in milliseconds.
	waitFor := func(t *testing.T, what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for %s", what)
	}

	t.Run("delivers to the registered handler", func(t *testing.T) {
		p := setup(t)
		var got atomic.Value
		p.Worker.Register("email:send", func(ctx context.Context, job queuec.Job) error {
			got.Store(string(job.Payload))
			return nil
		})
		start(t, p.Worker)
		if err := p.Enqueuer.Enqueue(context.Background(), queuec.Job{Type: "email:send", Payload: []byte("hi")}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "delivery", func() bool { v, _ := got.Load().(string); return v == "hi" })
	})

	t.Run("redelivers on failure until success", func(t *testing.T) {
		p := setup(t)
		var attempts atomic.Int32
		p.Worker.Register("flaky", func(ctx context.Context, job queuec.Job) error {
			if attempts.Add(1) < 3 {
				return errors.New("transient failure")
			}
			return nil
		})
		start(t, p.Worker)
		if err := p.Enqueuer.Enqueue(context.Background(), queuec.Job{Type: "flaky"}, queuec.Options{MaxRetry: 5}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "redelivery to succeed", func() bool { return attempts.Load() >= 3 })
	})

	t.Run("honors delay", func(t *testing.T) {
		p := setup(t)
		var handledAt atomic.Value
		p.Worker.Register("later", func(ctx context.Context, job queuec.Job) error {
			handledAt.Store(time.Now())
			return nil
		})
		start(t, p.Worker)
		enqueued := time.Now()
		if err := p.Enqueuer.Enqueue(context.Background(), queuec.Job{Type: "later"}, queuec.Options{Delay: 150 * time.Millisecond}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "delayed delivery", func() bool { return handledAt.Load() != nil })
		if early := handledAt.Load().(time.Time).Sub(enqueued); early < 100*time.Millisecond {
			t.Fatalf("delayed job ran after %s, want >= ~150ms", early)
		}
	})
}
