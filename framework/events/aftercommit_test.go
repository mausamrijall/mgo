package events_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mgo-framework/mgo/framework/events"
)

// fakeTx is a minimal orm.Transactor: it commits when fn returns nil and
// "rolls back" (does nothing durable) when fn errors. That is all the
// after-commit buffer depends on, so it exercises the real contract.
type fakeTx struct {
	commits   atomic.Int32
	rollbacks atomic.Int32
}

func (t *fakeTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := fn(ctx); err != nil {
		t.rollbacks.Add(1)
		return err
	}
	t.commits.Add(1)
	return nil
}

func TestCommitDeliversExactlyOnce(t *testing.T) {
	b := events.New()
	var delivered atomic.Int32
	events.Listen(b, func(ctx context.Context, e OrderPaid) error {
		delivered.Add(1)
		return nil
	})

	tx := &fakeTx{}
	err := b.Transaction(context.Background(), tx, func(ctx context.Context) error {
		// Emit several events; none should fire yet.
		events.DispatchAfterCommit(ctx, b, OrderPaid{ID: 1})
		events.DispatchAfterCommit(ctx, b, OrderPaid{ID: 2})
		if delivered.Load() != 0 {
			t.Fatal("events delivered before commit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tx.commits.Load() != 1 {
		t.Fatalf("commits = %d, want 1", tx.commits.Load())
	}
	if delivered.Load() != 2 {
		t.Fatalf("delivered = %d, want 2 (exactly once each)", delivered.Load())
	}
}

func TestRollbackDropsEvents(t *testing.T) {
	b := events.New()
	var delivered atomic.Int32
	events.Listen(b, func(ctx context.Context, e OrderPaid) error {
		delivered.Add(1)
		return nil
	})

	tx := &fakeTx{}
	boom := errors.New("business rule failed")
	err := b.Transaction(context.Background(), tx, func(ctx context.Context) error {
		events.DispatchAfterCommit(ctx, b, OrderPaid{ID: 1})
		return boom // rollback
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if tx.rollbacks.Load() != 1 {
		t.Fatalf("rollbacks = %d, want 1", tx.rollbacks.Load())
	}
	if delivered.Load() != 0 {
		t.Fatalf("delivered = %d after rollback, want 0 (events must be dropped)", delivered.Load())
	}
}

func TestAfterCommitWithoutTxIsImmediate(t *testing.T) {
	b := events.New()
	var delivered atomic.Int32
	events.Listen(b, func(ctx context.Context, e OrderPaid) error {
		delivered.Add(1)
		return nil
	})
	// No Bus.Transaction wrapping: behaves like Dispatch.
	if err := events.DispatchAfterCommit(context.Background(), b, OrderPaid{ID: 1}); err != nil {
		t.Fatal(err)
	}
	if delivered.Load() != 1 {
		t.Fatalf("delivered = %d, want 1 (immediate outside a tx)", delivered.Load())
	}
}
