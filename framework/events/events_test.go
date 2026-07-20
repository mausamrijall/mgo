package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/events"
	"github.com/mgo-framework/mgo/framework/queue"
)

// UserRegistered is a plain event value; no base type to embed.
type UserRegistered struct {
	Email string `json:"email"`
}

// OrderPaid names itself for stable wire routing.
type OrderPaid struct {
	ID int `json:"id"`
}

func (OrderPaid) EventName() string { return "order.paid" }

func TestSyncDispatchAndPriority(t *testing.T) {
	b := events.New()
	var order []string
	events.Listen(b, func(ctx context.Context, e UserRegistered) error {
		order = append(order, "low")
		return nil
	}, events.Priority(1))
	events.Listen(b, func(ctx context.Context, e UserRegistered) error {
		order = append(order, "high")
		return nil
	}, events.Priority(10))
	events.Listen(b, func(ctx context.Context, e UserRegistered) error {
		order = append(order, "mid")
		return nil
	}, events.Priority(5))

	if err := events.Dispatch(context.Background(), b, UserRegistered{Email: "a@b.c"}); err != nil {
		t.Fatal(err)
	}
	if got := order; len(got) != 3 || got[0] != "high" || got[1] != "mid" || got[2] != "low" {
		t.Fatalf("priority order = %v, want [high mid low]", got)
	}
}

func TestListenerErrorStopsChain(t *testing.T) {
	b := events.New()
	boom := errors.New("boom")
	ran := 0
	events.Listen(b, func(ctx context.Context, e UserRegistered) error { return boom }, events.Priority(10))
	events.Listen(b, func(ctx context.Context, e UserRegistered) error { ran++; return nil }, events.Priority(1))

	if err := events.Dispatch(context.Background(), b, UserRegistered{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if ran != 0 {
		t.Fatal("listener after the failing one still ran")
	}
}

func TestDistinctEventTypesAreIsolated(t *testing.T) {
	b := events.New()
	var users, orders int
	events.Listen(b, func(ctx context.Context, e UserRegistered) error { users++; return nil })
	events.Listen(b, func(ctx context.Context, e OrderPaid) error { orders++; return nil })

	events.Dispatch(context.Background(), b, UserRegistered{})
	events.Dispatch(context.Background(), b, OrderPaid{ID: 1})
	events.Dispatch(context.Background(), b, OrderPaid{ID: 2})
	if users != 1 || orders != 2 {
		t.Fatalf("users=%d orders=%d, want 1/2", users, orders)
	}
}

// TestQueuedListenerE2E: a queued listener runs in the worker, not
// inline — the Phase 8 queued-listener exit.
func TestQueuedListenerE2E(t *testing.T) {
	q := queue.NewMemory(2, 3)
	b := events.New(events.WithQueue(q, q))

	var mu sync.Mutex
	var got []string
	done := make(chan struct{}, 2)
	events.Listen(b, func(ctx context.Context, e OrderPaid) error {
		mu.Lock()
		got = append(got, "queued")
		mu.Unlock()
		done <- struct{}{}
		return nil
	}, events.Queued())
	events.Listen(b, func(ctx context.Context, e OrderPaid) error {
		mu.Lock()
		got = append(got, "sync")
		mu.Unlock()
		return nil
	}, events.Priority(10))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Run(ctx)

	if err := events.Dispatch(ctx, b, OrderPaid{ID: 7}); err != nil {
		t.Fatal(err)
	}
	// Sync listener ran inline already.
	mu.Lock()
	if len(got) != 1 || got[0] != "sync" {
		mu.Unlock()
		t.Fatalf("after dispatch got %v, want [sync] (queued runs later)", got)
	}
	mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("queued listener never ran in the worker")
	}
}

func TestQueuedWithoutQueuePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Queued() on a bus without a queue must panic")
		}
	}()
	b := events.New()
	events.Listen(b, func(ctx context.Context, e OrderPaid) error { return nil }, events.Queued())
}
