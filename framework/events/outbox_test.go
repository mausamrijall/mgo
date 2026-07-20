package events_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/events"
)

// memOutbox is an in-memory Outbox: enough to test the Relay's
// drain/deliver/mark loop without a database. A real outbox backs these
// three methods with a DB table via a Phase 4 orm adapter.
type memOutbox struct {
	mu        sync.Mutex
	envs      []events.Envelope
	delivered map[string]bool
}

func newMemOutbox() *memOutbox { return &memOutbox{delivered: map[string]bool{}} }

func (o *memOutbox) Add(ctx context.Context, env events.Envelope) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.envs = append(o.envs, env)
	return nil
}

func (o *memOutbox) Pending(ctx context.Context, limit int) ([]events.Envelope, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []events.Envelope
	for _, e := range o.envs {
		if !o.delivered[e.ID] {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (o *memOutbox) MarkDelivered(ctx context.Context, ids ...string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, id := range ids {
		o.delivered[id] = true
	}
	return nil
}

func TestOutboxRelayDeliversEachOnce(t *testing.T) {
	o := newMemOutbox()
	events.ToOutbox(context.Background(), o, OrderPaid{ID: 1})
	events.ToOutbox(context.Background(), o, OrderPaid{ID: 2})
	events.ToOutbox(context.Background(), o, OrderPaid{ID: 3})

	var deliveries sync.Map
	var count atomic.Int32
	relay := events.NewRelay(o, func(ctx context.Context, env events.Envelope) error {
		if _, dup := deliveries.LoadOrStore(env.ID, true); dup {
			t.Errorf("envelope %s delivered twice", env.ID)
		}
		count.Add(1)
		return nil
	}, 20*time.Millisecond, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); relay.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && count.Load() < 3 {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if count.Load() != 3 {
		t.Fatalf("delivered %d envelopes, want 3", count.Load())
	}
	// All marked delivered → a fresh drain finds nothing.
	pending, _ := o.Pending(context.Background(), 100)
	if len(pending) != 0 {
		t.Fatalf("%d envelopes still pending after delivery", len(pending))
	}
}

func TestOutboxRelayLeavesFailedPending(t *testing.T) {
	o := newMemOutbox()
	events.ToOutbox(context.Background(), o, OrderPaid{ID: 1})

	var attempts atomic.Int32
	relay := events.NewRelay(o, func(ctx context.Context, env events.Envelope) error {
		if attempts.Add(1) == 1 {
			return context.DeadlineExceeded // fail the first delivery
		}
		return nil
	}, 20*time.Millisecond, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); relay.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && attempts.Load() < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	if attempts.Load() < 2 {
		t.Fatalf("failed envelope not retried (attempts=%d)", attempts.Load())
	}
	pending, _ := o.Pending(context.Background(), 100)
	if len(pending) != 0 {
		t.Fatal("envelope not delivered on retry")
	}
}
