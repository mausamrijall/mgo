package events

import (
	"context"
	"sync"

	ormc "github.com/mgo-framework/mgo/contracts/orm"
)

// bufferKey carries the after-commit buffer in a transaction's context.
type bufferKeyT struct{}

var bufferKey bufferKeyT

// buffer collects events emitted inside a transaction and flushes them
// once, after commit. It never flushes on rollback.
type buffer struct {
	mu       sync.Mutex
	pending  []func(ctx context.Context) error
	flushed  bool
}

func (buf *buffer) add(fn func(ctx context.Context) error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	buf.pending = append(buf.pending, fn)
}

// flush runs each buffered dispatch exactly once. Idempotent: a second
// call (or one after rollback discarded the buffer) does nothing.
func (buf *buffer) flush(ctx context.Context) error {
	buf.mu.Lock()
	if buf.flushed {
		buf.mu.Unlock()
		return nil
	}
	buf.flushed = true
	pending := buf.pending
	buf.pending = nil
	buf.mu.Unlock()

	for _, fn := range pending {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Transaction runs fn inside tx (contracts/orm.Transactor) with an
// after-commit event buffer active. Events sent with DispatchAfterCommit
// inside fn are held until the transaction COMMITS, then dispatched
// exactly once. If fn returns an error (or panics), the transaction rolls
// back and the buffered events are DROPPED — never delivered.
//
// This is the txn-aware dispatch the roadmap's exactly-once tests target:
// listeners only see events for state that actually persisted.
func (b *Bus) Transaction(ctx context.Context, tx ormc.Transactor, fn func(ctx context.Context) error) error {
	buf := &buffer{}
	txCtx := context.WithValue(ctx, bufferKey, buf)

	err := tx.InTx(txCtx, func(innerCtx context.Context) error {
		// Ensure the buffer is reachable even if the Transactor swaps the
		// context (adapters add their tx handle to it).
		return fn(context.WithValue(innerCtx, bufferKey, buf))
	})
	if err != nil {
		return err // rollback happened; buffer is discarded, nothing flushed
	}
	// Commit succeeded: deliver on a clean context (the tx one is done).
	return buf.flush(context.WithoutCancel(ctx))
}

// DispatchAfterCommit dispatches event when the surrounding
// Bus.Transaction commits. Outside a transaction it dispatches
// immediately, so the same call site is correct in both settings.
func DispatchAfterCommit[E any](ctx context.Context, b *Bus, event E) error {
	buf, ok := ctx.Value(bufferKey).(*buffer)
	if !ok {
		return Dispatch(ctx, b, event) // no active transaction
	}
	buf.add(func(flushCtx context.Context) error { return Dispatch(flushCtx, b, event) })
	return nil
}
