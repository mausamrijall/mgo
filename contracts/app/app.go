// Package app defines the MGO application lifecycle contracts: providers,
// runners, and the hooks that adapters use to join boot/shutdown ordering.
package app

import (
	"context"

	"github.com/mgo-framework/mgo/contracts/config"
	"github.com/mgo-framework/mgo/contracts/container"
)

// Provider wires a capability into the application. Everything — adapters,
// modules, plugins, application services — registers through this one
// mechanism. Register must only bind into the container; resolution happens
// no earlier than Boot.
type Provider interface {
	// Register binds services into the container. It must not resolve.
	Register(app App) error
}

// Bootable providers run after ALL providers have registered, in
// registration order. This is where connections are established, routes
// mounted, and configuration validated against live dependencies.
type Bootable interface {
	Boot(ctx context.Context, app App) error
}

// Closable providers participate in graceful shutdown, called in reverse
// boot order with a deadline context.
type Closable interface {
	Close(ctx context.Context) error
}

// Deferrable providers register lazily: Register runs the first time one
// of the keys returned by Provides is resolved, not during boot. Provides
// returns typed nil pointers like (*T)(nil). A Deferrable provider must
// not also be Bootable — deferred work has no boot slot; the kernel
// rejects the combination.
type Deferrable interface {
	Provider
	Provides() []any
}

// Runner is a long-running unit (HTTP server, queue worker, scheduler)
// started by the app after boot. Run must block until ctx is cancelled or
// a fatal error occurs; returning nil after cancellation is a clean stop.
type Runner interface {
	// Name identifies the runner in logs and diagnostics.
	Name() string
	// Run blocks serving until ctx is cancelled.
	Run(ctx context.Context) error
}

// App is the surface providers see. It intentionally exposes only the
// kernel: container, config, and runner registration.
type App interface {
	Container() container.Container
	Config() config.Config
	// AddRunner registers a long-running unit started by Run.
	AddRunner(r Runner)
}
