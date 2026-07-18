// Package mgo is the MGO application kernel: lifecycle (New → Boot → Run →
// Shutdown), provider ordering, runner supervision, and signal handling.
//
// MGO is a Go Application Platform — a DX layer over the Go ecosystem. This
// kernel is deliberately small, stable, and stdlib-only; capabilities join
// through providers and runners (contracts/app).
package mgo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
	"github.com/mgo-framework/mgo/contracts/config"
	"github.com/mgo-framework/mgo/contracts/container"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/di"
)

// Option configures the App builder.
type Option func(*App)

// WithConfig replaces the default config loader result.
func WithConfig(cfg config.Config) Option {
	return func(a *App) { a.cfg = cfg }
}

// WithProviders appends providers in registration order.
func WithProviders(ps ...appc.Provider) Option {
	return func(a *App) { a.providers = append(a.providers, ps...) }
}

// WithShutdownTimeout sets the graceful shutdown deadline (default 30s).
func WithShutdownTimeout(d time.Duration) Option {
	return func(a *App) { a.shutdownTimeout = d }
}

// WithLogger replaces the kernel logger.
func WithLogger(l *slog.Logger) Option {
	return func(a *App) { a.log = l }
}

// App is the MGO application kernel.
type App struct {
	c         *di.Container
	cfg       config.Config
	log       *slog.Logger
	providers []appc.Provider
	runners   []appc.Runner
	booted    []appc.Closable // successfully booted closables, boot order

	shutdownTimeout time.Duration

	mu    sync.Mutex
	phase string // "new" → "booted" → "running" → "stopped"
}

var _ appc.App = (*App)(nil)

// New builds an App. Provider registration and boot are deferred to Boot,
// which Run calls automatically if needed.
func New(opts ...Option) *App {
	a := &App{
		c:               di.New(),
		log:             slog.Default(),
		shutdownTimeout: 30 * time.Second,
		phase:           "new",
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.cfg == nil {
		// Canonical default chain; apps with files add them via WithConfig.
		cfg, err := conf.NewLoader().DotEnv(".env", true).Env("MGO_").Load()
		if err != nil {
			// .env is optional; env source cannot fail. Defensive anyway.
			cfg, _ = conf.NewLoader().Load()
			a.log.Warn("mgo: default config load failed", "error", err)
		}
		a.cfg = cfg
	}
	return a
}

// Container implements contracts/app.App.
func (a *App) Container() container.Container { return a.c }

// Config implements contracts/app.App.
func (a *App) Config() config.Config { return a.cfg }

// Log returns the kernel logger.
func (a *App) Log() *slog.Logger { return a.log }

// AddRunner implements contracts/app.App.
func (a *App) AddRunner(r appc.Runner) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runners = append(a.runners, r)
}

// Boot runs the register phase for all providers, seeds kernel bindings,
// validates the container graph, then runs Boot on bootable providers in
// registration order. Boot is idempotent.
func (a *App) Boot(ctx context.Context) error {
	a.mu.Lock()
	if a.phase != "new" {
		a.mu.Unlock()
		return nil
	}
	a.phase = "booting"
	a.mu.Unlock()

	// Kernel self-bindings: providers can inject these.
	if err := errors.Join(
		di.Instance[config.Config](a.c, a.cfg),
		di.Instance[*slog.Logger](a.c, a.log),
		di.Instance[container.Container](a.c, a.c),
	); err != nil {
		return fmt.Errorf("mgo: kernel bindings: %w", err)
	}

	for _, p := range a.providers {
		if d, ok := p.(appc.Deferrable); ok {
			if _, bootable := p.(appc.Bootable); bootable {
				return fmt.Errorf("mgo: provider %T is both Deferrable and Bootable; deferred providers have no boot slot", p)
			}
			if err := a.c.Defer(func() error { return d.Register(a) }, d.Provides()...); err != nil {
				return fmt.Errorf("mgo: defer %T: %w", p, err)
			}
			continue
		}
		if err := p.Register(a); err != nil {
			return fmt.Errorf("mgo: register %T: %w", p, err)
		}
	}
	if err := a.c.Validate(); err != nil {
		return fmt.Errorf("mgo: boot: %w", err)
	}
	for _, p := range a.providers {
		b, ok := p.(appc.Bootable)
		if !ok {
			continue
		}
		if err := b.Boot(ctx, a); err != nil {
			// Unwind already-booted providers before failing.
			a.closeBooted(ctx)
			return fmt.Errorf("mgo: boot %T: %w", p, err)
		}
		if cl, ok := p.(appc.Closable); ok {
			a.booted = append(a.booted, cl)
		}
	}

	a.mu.Lock()
	a.phase = "booted"
	a.mu.Unlock()
	return nil
}

// Run boots if necessary, starts all runners, and blocks until ctx is
// cancelled, a SIGINT/SIGTERM arrives, or a runner fails. It then performs
// graceful shutdown and returns the first fatal error, if any.
func (a *App) Run(ctx context.Context) error {
	if err := a.Boot(ctx); err != nil {
		return err
	}

	runCtx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	a.mu.Lock()
	a.phase = "running"
	runners := a.runners
	a.mu.Unlock()

	errs := make(chan error, len(runners))
	var wg sync.WaitGroup
	for _, r := range runners {
		wg.Add(1)
		go func(r appc.Runner) {
			defer wg.Done()
			a.log.Info("mgo: runner starting", "runner", r.Name())
			if err := r.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				errs <- fmt.Errorf("runner %s: %w", r.Name(), err)
				cancel() // one fatal runner stops the app
			}
		}(r)
	}

	<-runCtx.Done()
	a.log.Info("mgo: shutting down", "timeout", a.shutdownTimeout)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	shutdownCtx, sdCancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer sdCancel()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		a.log.Warn("mgo: runners did not stop before deadline")
	}

	a.closeBooted(shutdownCtx)

	a.mu.Lock()
	a.phase = "stopped"
	a.mu.Unlock()

	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

// closeBooted closes bootable providers in reverse boot order.
func (a *App) closeBooted(ctx context.Context) {
	for i := len(a.booted) - 1; i >= 0; i-- {
		if err := a.booted[i].Close(ctx); err != nil {
			a.log.Error("mgo: provider close failed", "provider", fmt.Sprintf("%T", a.booted[i]), "error", err)
		}
	}
	a.booted = nil
}

// ProviderFunc adapts a function to appc.Provider.
type ProviderFunc func(app appc.App) error

// Register implements contracts/app.Provider.
func (f ProviderFunc) Register(app appc.App) error { return f(app) }
