// Package httpserver is the kernel's stdlib HTTP runner: it adapts any
// http.Handler into the app lifecycle with graceful shutdown. Router
// adapters (chi, gin, echo, stdmux) produce the http.Handler; this runner
// is deliberately handler-agnostic — the glue philosophy at work.
package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
)

// Config for the HTTP runner; bound from the "http" config section.
type Config struct {
	Addr              string        `conf:"addr"`
	ReadHeaderTimeout time.Duration `conf:"read_header_timeout"`
	ShutdownGrace     time.Duration `conf:"shutdown_grace"`
}

// Runner serves an http.Handler as an app runner.
type Runner struct {
	name    string
	handler http.Handler
	cfg     Config

	// ready is closed once the listener is bound (tests use Addr after this).
	ready chan struct{}
	addr  string
}

var _ appc.Runner = (*Runner)(nil)

// New creates an HTTP runner. Zero-value config fields get safe defaults
// (addr ":8080", read header timeout 10s, shutdown grace 15s).
func New(name string, handler http.Handler, cfg Config) *Runner {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.ReadHeaderTimeout == 0 {
		cfg.ReadHeaderTimeout = 10 * time.Second
	}
	if cfg.ShutdownGrace == 0 {
		cfg.ShutdownGrace = 15 * time.Second
	}
	return &Runner{name: name, handler: handler, cfg: cfg, ready: make(chan struct{})}
}

// Name implements contracts/app.Runner.
func (r *Runner) Name() string { return r.name }

// Addr returns the bound address once the listener is up (blocks until
// ready or ctx done). Useful with "addr: :0" in tests.
func (r *Runner) Addr(ctx context.Context) (string, error) {
	select {
	case <-r.ready:
		return r.addr, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Run implements contracts/app.Runner: serve until ctx cancels, then
// gracefully drain in-flight requests within ShutdownGrace.
func (r *Runner) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", r.cfg.Addr)
	if err != nil {
		return err
	}
	r.addr = ln.Addr().String()
	close(r.ready)

	srv := &http.Server{
		Handler:           r.handler,
		ReadHeaderTimeout: r.cfg.ReadHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err // listener failure
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.Background(), r.cfg.ShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(grace); err != nil {
			_ = srv.Close()
			return err
		}
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
