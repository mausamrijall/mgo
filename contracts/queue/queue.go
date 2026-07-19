// Package queue defines MGO's background-job integration points: enqueue
// a typed payload, register a handler, run workers inside the app
// lifecycle. Brokers (asynq/River/rabbitmq/nats) are adapters; payloads
// are raw bytes so codecs stay the application's choice.
package queue

import (
	"context"
	"time"
)

// Job is a unit of background work.
type Job struct {
	// Type routes the job to its registered handler.
	Type string
	// Payload is the encoded job data (JSON, proto — your call).
	Payload []byte
}

// Options tune a single enqueue. The zero value means: now, with the
// driver's default retry policy.
type Options struct {
	// Delay schedules the job to run no earlier than now+Delay.
	Delay time.Duration
	// MaxRetry caps redelivery attempts after failures (0 = driver default).
	MaxRetry int
}

// Enqueuer is the producer side.
type Enqueuer interface {
	Enqueue(ctx context.Context, job Job, opts ...Options) error
}

// Handler processes one job. A non-nil error means failure: the driver
// redelivers (up to its retry policy). Handlers must be idempotent —
// at-least-once delivery is the contract.
type Handler func(ctx context.Context, job Job) error

// Worker is the consumer side. Register wires handlers by job type
// before Run; Run blocks consuming until ctx cancels, then drains
// in-flight jobs. Workers plug into the app as contracts/app.Runner.
type Worker interface {
	Register(jobType string, h Handler)
	Run(ctx context.Context) error
}
