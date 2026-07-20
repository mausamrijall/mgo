package mgotest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ormc "github.com/mgo-framework/mgo/contracts/orm"
	queuec "github.com/mgo-framework/mgo/contracts/queue"
	"github.com/mgo-framework/mgo/framework/events"
)

// QueueRecorder is a contracts/queue.Enqueuer that records jobs instead
// of executing them — for asserting THAT something was enqueued. To
// actually run handlers in a test, use framework/queue.NewMemory.
type QueueRecorder struct {
	mu   sync.Mutex
	jobs []queuec.Job
}

var _ queuec.Enqueuer = (*QueueRecorder)(nil)

// NewQueueRecorder returns an empty recorder.
func NewQueueRecorder() *QueueRecorder { return &QueueRecorder{} }

// Enqueue implements contracts/queue.Enqueuer.
func (q *QueueRecorder) Enqueue(ctx context.Context, job queuec.Job, opts ...queuec.Options) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
	return nil
}

// Jobs returns every recorded job.
func (q *QueueRecorder) Jobs() []queuec.Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]queuec.Job(nil), q.jobs...)
}

// JobsOf returns recorded jobs of one type.
func (q *QueueRecorder) JobsOf(jobType string) []queuec.Job {
	var out []queuec.Job
	for _, j := range q.Jobs() {
		if j.Type == jobType {
			out = append(out, j)
		}
	}
	return out
}

// Recorded collects events of type E captured by RecordEvents.
type Recorded[E any] struct {
	mu    sync.Mutex
	items []E
	ch    chan E
}

// RecordEvents registers a listener capturing every dispatched E —
// synchronous or flushed-after-commit — for later assertion.
func RecordEvents[E any](b *events.Bus) *Recorded[E] {
	rec := &Recorded[E]{ch: make(chan E, 128)}
	events.Listen(b, func(ctx context.Context, e E) error {
		rec.mu.Lock()
		rec.items = append(rec.items, e)
		rec.mu.Unlock()
		select {
		case rec.ch <- e:
		default:
		}
		return nil
	})
	return rec
}

// All returns every captured event so far.
func (r *Recorded[E]) All() []E {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]E(nil), r.items...)
}

// Wait blocks until n events have been captured (or fails the test after
// timeout) — for asserting async paths like queued listeners.
func (r *Recorded[E]) Wait(t testing.TB, n int, timeout time.Duration) []E {
	t.Helper()
	deadline := time.After(timeout)
	for {
		r.mu.Lock()
		count := len(r.items)
		r.mu.Unlock()
		if count >= n {
			return r.All()
		}
		select {
		case <-r.ch:
		case <-deadline:
			t.Fatalf("mgotest: captured %d events, want %d within %s", count, n, timeout)
			return nil
		}
	}
}

// errRollback is the sentinel InRollback uses to force a rollback.
var errRollback = errors.New("mgotest: intentional rollback")

// InRollback runs fn inside a transaction that ALWAYS rolls back —
// database state is untouched no matter what fn writes. Works with any
// contracts/orm.Transactor (db-sql, orm-gorm, orm-ent).
func InRollback(t testing.TB, tx ormc.Transactor, fn func(ctx context.Context)) {
	t.Helper()
	err := tx.InTx(context.Background(), func(ctx context.Context) error {
		fn(ctx)
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatalf("mgotest: transaction: %v", err)
	}
}
