// Package mgoasynq adapts hibiken/asynq to MGO's queue contract. The
// asynq client and server are embedded/exposed — asynq's native API
// (queues, priorities, schedulers, inspectors) stays fully available;
// MGO adds only the contract shape and app-lifecycle integration.
package mgoasynq

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	appc "github.com/mgo-framework/mgo/contracts/app"
	queuec "github.com/mgo-framework/mgo/contracts/queue"
)

// Client is the producer: an embedded *asynq.Client implementing
// contracts/queue.Enqueuer.
type Client struct {
	*asynq.Client
}

var _ queuec.Enqueuer = (*Client)(nil)

// NewClient connects a producer to redis.
func NewClient(redis asynq.RedisConnOpt) *Client {
	return &Client{Client: asynq.NewClient(redis)}
}

// Enqueue implements contracts/queue.Enqueuer.
func (c *Client) Enqueue(ctx context.Context, job queuec.Job, opts ...queuec.Options) error {
	var o queuec.Options
	if len(opts) > 0 {
		o = opts[0]
	}
	var aopts []asynq.Option
	if o.Delay > 0 {
		aopts = append(aopts, asynq.ProcessIn(o.Delay))
	}
	if o.MaxRetry > 0 {
		aopts = append(aopts, asynq.MaxRetry(o.MaxRetry))
	}
	_, err := c.Client.EnqueueContext(ctx, asynq.NewTask(job.Type, job.Payload), aopts...)
	return err
}

// Worker is the consumer: an asynq server wrapped as an app Runner.
type Worker struct {
	srv *asynq.Server
	mux *asynq.ServeMux
}

var (
	_ queuec.Worker = (*Worker)(nil)
	_ appc.Runner   = (*Worker)(nil)
)

// Config tunes the worker; zero values get asynq defaults except
// Concurrency (default 4).
type Config struct {
	Concurrency int
	// RetryDelay overrides asynq's exponential backoff when > 0 — a flat
	// redelivery delay (tests use small values; production usually keeps
	// the default backoff by leaving it 0).
	RetryDelay time.Duration
	// DelayedTaskCheckInterval is how often asynq polls for due
	// delayed/retry tasks (asynq default 5s; tests set it small).
	DelayedTaskCheckInterval time.Duration
	// ShutdownTimeout bounds the drain of in-flight jobs (default 8s,
	// matching asynq).
	ShutdownTimeout time.Duration
}

// NewWorker builds a worker; Register handlers, then hand it to
// app.AddRunner (or call Run yourself).
func NewWorker(redis asynq.RedisConnOpt, cfg Config) *Worker {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	acfg := asynq.Config{
		Concurrency:              cfg.Concurrency,
		ShutdownTimeout:          cfg.ShutdownTimeout,
		DelayedTaskCheckInterval: cfg.DelayedTaskCheckInterval,
	}
	if cfg.RetryDelay > 0 {
		acfg.RetryDelayFunc = func(int, error, *asynq.Task) time.Duration { return cfg.RetryDelay }
	}
	return &Worker{srv: asynq.NewServer(redis, acfg), mux: asynq.NewServeMux()}
}

// Register implements contracts/queue.Worker.
func (w *Worker) Register(jobType string, h queuec.Handler) {
	w.mux.HandleFunc(jobType, func(ctx context.Context, t *asynq.Task) error {
		return h(ctx, queuec.Job{Type: t.Type(), Payload: t.Payload()})
	})
}

// Name implements contracts/app.Runner.
func (w *Worker) Name() string { return "queue-asynq" }

// Run implements contracts/app.Runner and contracts/queue.Worker: serve
// until ctx cancels, then shut down gracefully (asynq requeues what
// cannot drain in time — nothing is lost, at-least-once holds).
func (w *Worker) Run(ctx context.Context) error {
	if err := w.srv.Start(w.mux); err != nil {
		return err
	}
	<-ctx.Done()
	w.srv.Shutdown()
	return nil
}
