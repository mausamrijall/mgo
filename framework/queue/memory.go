// Package queue is MGO's first-party in-memory queue driver: full
// contract semantics (typed handlers, delay, retry with redelivery,
// graceful drain) with zero infrastructure. It is the dev/test driver;
// production uses a broker adapter (queue-asynq) behind the same
// contracts, so swapping is a constructor change.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
	queuec "github.com/mgo-framework/mgo/contracts/queue"
)

// Memory is an in-process queue: Enqueuer + Worker + app Runner.
type Memory struct {
	name        string
	concurrency int
	maxRetry    int
	log         *slog.Logger

	ch chan delivery

	mu       sync.RWMutex
	handlers map[string]queuec.Handler
	running  bool
}

type delivery struct {
	job     queuec.Job
	attempt int
	max     int
}

var (
	_ queuec.Enqueuer = (*Memory)(nil)
	_ queuec.Worker   = (*Memory)(nil)
	_ appc.Runner     = (*Memory)(nil)
)

// NewMemory builds a queue with the given worker concurrency (default 4)
// and default max retries per job (default 3).
func NewMemory(concurrency, maxRetry int) *Memory {
	if concurrency <= 0 {
		concurrency = 4
	}
	if maxRetry <= 0 {
		maxRetry = 3
	}
	return &Memory{
		name:        "queue-memory",
		concurrency: concurrency,
		maxRetry:    maxRetry,
		log:         slog.Default(),
		ch:          make(chan delivery, 1024),
		handlers:    map[string]queuec.Handler{},
	}
}

// Register implements contracts/queue.Worker.
func (m *Memory) Register(jobType string, h queuec.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[jobType] = h
}

// Enqueue implements contracts/queue.Enqueuer.
func (m *Memory) Enqueue(ctx context.Context, job queuec.Job, opts ...queuec.Options) error {
	var o queuec.Options
	if len(opts) > 0 {
		o = opts[0]
	}
	max := o.MaxRetry
	if max <= 0 {
		max = m.maxRetry
	}
	d := delivery{job: job, attempt: 1, max: max}
	if o.Delay > 0 {
		time.AfterFunc(o.Delay, func() { m.push(d) })
		return nil
	}
	m.push(d)
	return nil
}

func (m *Memory) push(d delivery) {
	defer func() { recover() }() // late timers after Close of ch: drop silently
	m.ch <- d
}

// Name implements contracts/app.Runner.
func (m *Memory) Name() string { return m.name }

// Run implements contracts/app.Runner: consume until ctx cancels, then
// finish in-flight jobs and return.
func (m *Memory) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for range m.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case d := <-m.ch:
					m.process(ctx, d)
				}
			}
		}()
	}
	wg.Wait()
	return nil
}

func (m *Memory) process(ctx context.Context, d delivery) {
	m.mu.RLock()
	h, ok := m.handlers[d.job.Type]
	m.mu.RUnlock()
	if !ok {
		m.log.Error("queue: no handler", "type", d.job.Type)
		return
	}
	err := safeCall(ctx, h, d.job)
	if err == nil {
		return
	}
	if d.attempt >= d.max {
		m.log.Error("queue: job dropped after max retries",
			"type", d.job.Type, "attempts", d.attempt, "error", err)
		return
	}
	d.attempt++
	// Redeliver with a small backoff so a hot failure doesn't spin.
	time.AfterFunc(10*time.Millisecond, func() { m.push(d) })
}

func safeCall(ctx context.Context, h queuec.Handler, job queuec.Job) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("queue: handler panic: %v", v)
		}
	}()
	return h(ctx, job)
}
