package events

import (
	"context"
	"log/slog"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
)

// Outbox is the transactional-outbox store: events written in the same
// transaction as business data (Add), later read and delivered by a
// Relay. Implementations back it with a DB table (via a Phase 4 orm
// adapter). Storing the event and the data atomically is what upgrades
// at-least-once delivery to effectively exactly-once end to end.
type Outbox interface {
	// Add records an envelope for later delivery. Called inside the
	// business transaction, using the tx handle in ctx.
	Add(ctx context.Context, env Envelope) error
	// Pending returns up to limit undelivered envelopes, oldest first.
	Pending(ctx context.Context, limit int) ([]Envelope, error)
	// MarkDelivered marks envelopes done by ID after successful delivery.
	MarkDelivered(ctx context.Context, ids ...string) error
}

// ToOutbox writes an event to the outbox as a CloudEvents envelope. Call
// it inside your business transaction so the event persists atomically
// with the data it describes.
func ToOutbox[E any](ctx context.Context, o Outbox, event E) error {
	return o.Add(ctx, NewEnvelope(nameOf(event), event))
}

// Relay drains an Outbox on an interval and delivers each envelope via
// deliver, then marks it done. It is an app Runner. Delivery is
// at-least-once (a crash after deliver, before MarkDelivered, redelivers)
// — deliver targets must be idempotent, keyed on Envelope.ID.
type Relay struct {
	outbox   Outbox
	deliver  func(ctx context.Context, env Envelope) error
	interval time.Duration
	batch    int
	log      *slog.Logger
}

var _ appc.Runner = (*Relay)(nil)

// NewRelay builds a relay. interval defaults to 1s, batch to 100.
func NewRelay(o Outbox, deliver func(ctx context.Context, env Envelope) error, interval time.Duration, batch int) *Relay {
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 100
	}
	return &Relay{outbox: o, deliver: deliver, interval: interval, batch: batch, log: slog.Default()}
}

// Name implements contracts/app.Runner.
func (r *Relay) Name() string { return "events-relay" }

// Run implements contracts/app.Runner: drain until ctx cancels, doing a
// final drain on the way out so shutdown doesn't strand delivered-able
// events.
func (r *Relay) Run(ctx context.Context) error {
	tick := time.NewTicker(r.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			r.drain(context.WithoutCancel(ctx))
			return nil
		case <-tick.C:
			r.drain(ctx)
		}
	}
}

// drain delivers one batch, logging (not returning) errors so a single
// bad envelope can't wedge the loop.
func (r *Relay) drain(ctx context.Context) {
	envs, err := r.outbox.Pending(ctx, r.batch)
	if err != nil {
		r.log.Error("events relay: pending", "error", err)
		return
	}
	var delivered []string
	for _, env := range envs {
		if err := r.deliver(ctx, env); err != nil {
			r.log.Error("events relay: deliver", "id", env.ID, "type", env.Type, "error", err)
			continue // leave it pending; retried next tick
		}
		delivered = append(delivered, env.ID)
	}
	if len(delivered) > 0 {
		if err := r.outbox.MarkDelivered(ctx, delivered...); err != nil {
			r.log.Error("events relay: mark delivered", "error", err)
		}
	}
}
