// Package events is MGO's first-party event dispatcher — one of the few
// genuinely first-party subsystems (doc 07: a real ecosystem gap, not a
// library to adapt). It is a typed, generic dispatcher with prioritized
// listeners, optional queued delivery over contracts/queue, and
// transaction-aware DispatchAfterCommit over contracts/orm.
//
// Events are ordinary Go values. Listeners are ordinary functions. There
// is no base-event type to embed and no framework Ctx — delete MGO and
// you are left with functions calling functions.
package events

import (
	"context"
	"log/slog"
	"reflect"
	"sort"
	"sync"

	queuec "github.com/mgo-framework/mgo/contracts/queue"
)

// NamedEvent lets an event choose its stable wire name (used for queued
// delivery and outbox routing). Events that don't implement it use their
// Go type name, which is fine for in-process dispatch.
type NamedEvent interface {
	EventName() string
}

// Listener handles an event of type E.
type Listener[E any] func(ctx context.Context, event E) error

// Bus dispatches events to registered listeners. The zero value is not
// usable; call New.
type Bus struct {
	mu        sync.RWMutex
	listeners map[string][]registration // event name → listeners (priority order)
	decoders  map[string]func([]byte) (any, error)

	enqueuer queuec.Enqueuer // nil unless WithQueue is configured
	worker   queuec.Worker
	log      *slog.Logger

	registeredQueue map[string]bool // event names with a queue handler wired
}

type registration struct {
	name     string
	priority int
	queued   bool
	call     func(ctx context.Context, event any) error
}

// Option configures the Bus.
type Option func(*Bus)

// WithQueue enables queued listeners: WithQueue's enqueuer receives jobs
// on dispatch, and its worker runs the queued listeners. Both ends
// usually come from one queue driver (framework/queue.Memory or
// adapters/queue-asynq).
func WithQueue(enqueuer queuec.Enqueuer, worker queuec.Worker) Option {
	return func(b *Bus) { b.enqueuer = enqueuer; b.worker = worker }
}

// WithLogger sets the logger for listener errors on the async paths.
func WithLogger(l *slog.Logger) Option {
	return func(b *Bus) { b.log = l }
}

// New builds a Bus.
func New(opts ...Option) *Bus {
	b := &Bus{
		listeners:       map[string][]registration{},
		decoders:        map[string]func([]byte) (any, error){},
		registeredQueue: map[string]bool{},
		log:             slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// nameOf returns an event value's wire name.
func nameOf(v any) string {
	if n, ok := v.(NamedEvent); ok {
		return n.EventName()
	}
	return reflect.TypeOf(v).String()
}

// nameFor returns type E's wire name without a value in hand.
func nameFor[E any]() string {
	var zero E
	if n, ok := any(zero).(NamedEvent); ok {
		return n.EventName()
	}
	return reflect.TypeOf(zero).String()
}

// Dispatch delivers event to every listener for its type. Synchronous
// listeners run inline in priority order (high → low); the first error
// stops the chain and is returned. Queued listeners are enqueued (one
// job per event) and run later in the worker.
func Dispatch[E any](ctx context.Context, b *Bus, event E) error {
	name := nameOf(event)
	b.mu.RLock()
	regs := b.listeners[name]
	enqueuer := b.enqueuer
	b.mu.RUnlock()

	hasQueued := false
	for _, r := range regs {
		if r.queued {
			hasQueued = true
			continue
		}
		if err := r.call(ctx, event); err != nil {
			return err
		}
	}

	if hasQueued && enqueuer != nil {
		payload, err := encodeEvent(event)
		if err != nil {
			return err
		}
		return enqueuer.Enqueue(ctx, queuec.Job{Type: jobPrefix + name, Payload: payload})
	}
	return nil
}

// sortListeners orders a name's registrations by descending priority,
// stable within equal priorities (registration order preserved).
func (b *Bus) sortListeners(name string) {
	regs := b.listeners[name]
	sort.SliceStable(regs, func(i, j int) bool { return regs[i].priority > regs[j].priority })
}
