package events

import (
	"context"
	"encoding/json"
	"fmt"

	queuec "github.com/mgo-framework/mgo/contracts/queue"
)

// jobPrefix namespaces queued-event jobs in the queue.
const jobPrefix = "event:"

// ListenOption tunes a listener registration.
type ListenOption func(*registration)

// Priority sets execution order for synchronous listeners: higher runs
// first (default 0). Ties keep registration order.
func Priority(p int) ListenOption {
	return func(r *registration) { r.priority = p }
}

// Named labels the listener in logs and diagnostics.
func Named(name string) ListenOption {
	return func(r *registration) { r.name = name }
}

// Queued marks the listener for asynchronous delivery through the queue
// configured with WithQueue. The event is JSON-encoded on dispatch and
// the listener runs in the worker. Panics if the bus has no queue —
// that is a wiring error, not a runtime condition.
func Queued() ListenOption {
	return func(r *registration) { r.queued = true }
}

// Listen registers a listener for events of type E. Register all
// listeners during boot (before Dispatch); registration is not designed
// for concurrent use with dispatch.
func Listen[E any](b *Bus, listener Listener[E], opts ...ListenOption) {
	name := nameFor[E]()
	reg := registration{
		name: fmt.Sprintf("%s#%d", name, len(b.listeners[name])),
		call: func(ctx context.Context, event any) error {
			e, ok := event.(E)
			if !ok {
				return fmt.Errorf("events: listener for %s got %T", name, event)
			}
			return listener(ctx, e)
		},
	}
	for _, opt := range opts {
		opt(&reg)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if reg.queued {
		if b.enqueuer == nil || b.worker == nil {
			panic("events: Queued() listener registered but the bus has no queue (use events.WithQueue)")
		}
		// One decoder + one queue handler per event name, wired once.
		if !b.registeredQueue[name] {
			b.decoders[name] = decoderFor[E]()
			b.worker.Register(jobPrefix+name, b.runQueued(name))
			b.registeredQueue[name] = true
		}
	}

	b.listeners[name] = append(b.listeners[name], reg)
	b.sortListeners(name)
}

// runQueued returns the queue handler that decodes an event and runs its
// queued listeners. A listener error fails the job so the queue retries
// (at-least-once — queued listeners must be idempotent, like any job).
func (b *Bus) runQueued(name string) queuec.Handler {
	return func(ctx context.Context, job queuec.Job) error {
		b.mu.RLock()
		decode := b.decoders[name]
		regs := append([]registration(nil), b.listeners[name]...)
		b.mu.RUnlock()

		event, err := decodeEvent(job.Payload, decode)
		if err != nil {
			return err
		}
		for _, r := range regs {
			if !r.queued {
				continue
			}
			if err := r.call(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
}

// encodeEvent wraps an event in a CloudEvents envelope for the wire.
func encodeEvent[E any](event E) ([]byte, error) {
	return json.Marshal(NewEnvelope(nameOf(event), event))
}

// decodeEvent unwraps the envelope and decodes the data via the
// type-specific decoder registered at Listen time.
func decodeEvent(payload []byte, decode func([]byte) (any, error)) (any, error) {
	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, err
	}
	return decode(env.Data)
}

func decoderFor[E any]() func([]byte) (any, error) {
	return func(raw []byte) (any, error) {
		var e E
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		return e, nil
	}
}
