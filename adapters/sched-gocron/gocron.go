// Package mgogocron adapts go-co-op/gocron to the MGO app lifecycle. The
// gocron.Scheduler is embedded — its native API (cron expressions,
// monthly jobs, listeners) stays fully available. MGO adds the Every
// helper with two production behaviors:
//
//   - WithoutOverlapping: a slow run never overlaps the next tick
//     (gocron singleton mode, in-process).
//   - OnOneServer: across a fleet sharing a cache Locker (cache-redis in
//     production), each tick runs on exactly one instance.
package mgogocron

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
	appc "github.com/mgo-framework/mgo/contracts/app"
	cachec "github.com/mgo-framework/mgo/contracts/cache"
)

// Scheduler wraps gocron as a contracts/app.Runner.
type Scheduler struct {
	gocron.Scheduler
	log *slog.Logger
}

var _ appc.Runner = (*Scheduler)(nil)

// New builds a scheduler; gocron options pass through untouched.
func New(opts ...gocron.SchedulerOption) (*Scheduler, error) {
	s, err := gocron.NewScheduler(opts...)
	if err != nil {
		return nil, err
	}
	return &Scheduler{Scheduler: s, log: slog.Default()}, nil
}

type jobConfig struct {
	singleton bool
	locker    cachec.Locker
	lockTTL   time.Duration
}

// JobOption tunes one scheduled job.
type JobOption func(*jobConfig)

// WithoutOverlapping skips a tick while the previous run is still going
// (per process).
func WithoutOverlapping() JobOption {
	return func(c *jobConfig) { c.singleton = true }
}

// OnOneServer runs each tick on at most one instance among all that
// share the locker. ttl should cover roughly one interval; the lock is
// left to expire so the winner holds the tick exclusively.
func OnOneServer(l cachec.Locker, ttl time.Duration) JobOption {
	return func(c *jobConfig) { c.locker = l; c.lockTTL = ttl }
}

// Every schedules fn on a fixed interval. name identifies the job in
// logs and as the distributed-lock key.
func (s *Scheduler) Every(interval time.Duration, name string, fn func(context.Context) error, opts ...JobOption) error {
	var cfg jobConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	run := fn
	if cfg.locker != nil {
		ttl := cfg.lockTTL
		if ttl <= 0 {
			ttl = interval
		}
		inner := run
		run = func(ctx context.Context) error {
			_, ok, err := cfg.locker.TryLock(ctx, "sched:"+name, ttl)
			if err != nil {
				return err
			}
			if !ok {
				return nil // another instance won this tick
			}
			return inner(ctx) // lock expires on its own: the tick stays owned
		}
	}

	gopts := []gocron.JobOption{gocron.WithName(name)}
	if cfg.singleton {
		gopts = append(gopts, gocron.WithSingletonMode(gocron.LimitModeReschedule))
	}

	logged := func() {
		if err := run(context.Background()); err != nil {
			s.log.Error("scheduler: job failed", "job", name, "error", err)
		}
	}
	_, err := s.NewJob(gocron.DurationJob(interval), gocron.NewTask(logged), gopts...)
	return err
}

// Name implements contracts/app.Runner.
func (s *Scheduler) Name() string { return "scheduler" }

// Run implements contracts/app.Runner: start ticking, stop gracefully
// when the app shuts down (running jobs finish).
func (s *Scheduler) Run(ctx context.Context) error {
	s.Start()
	<-ctx.Done()
	return s.Shutdown()
}
