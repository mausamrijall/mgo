// Package container defines the MGO service container contracts.
//
// The container is one of the few first-party components of the MGO kernel
// (DDR D21): it resolves the application's dependency graph at boot time and
// performs zero reflection on the hot path thereafter.
package container

import "context"

// Lifetime controls how long a resolved instance lives.
type Lifetime int

const (
	// Transient bindings produce a new instance on every resolve.
	Transient Lifetime = iota
	// Singleton bindings produce one lazily-built instance per root container.
	Singleton
	// Scoped bindings produce one instance per Scope (HTTP request, job run,
	// test case, ...). Resolving a Scoped binding from the root is an error.
	Scoped
)

// Container is the root registration and resolution surface.
//
// Registration happens during the app's register phase; the graph is
// validated during boot. Constructors are plain Go functions whose
// parameters are resolved recursively; they may optionally return an
// error as their second return value.
type Container interface {
	Resolver

	// Bind registers constructor for the type key with the given lifetime.
	// key must be a pointer to the bound type (possibly nil), e.g.
	// (*UserRepository)(nil). Prefer the generic helpers in framework/di.
	Bind(key any, constructor any, lifetime Lifetime) error

	// Instance registers an already-built value as a singleton.
	Instance(key any, value any) error

	// Validate checks the whole graph for missing bindings, dependency
	// cycles and lifetime violations (e.g. Singleton depending on Scoped).
	// The kernel calls this during boot; errors carry the full chain.
	Validate() error
}

// Resolver resolves previously registered bindings.
type Resolver interface {
	// Resolve returns the instance bound to key.
	Resolve(key any) (any, error)

	// Call invokes fn, resolving each parameter from the container.
	// fn may return (error), (T), or (T, error).
	Call(fn any) ([]any, error)

	// Scope opens a child scope. Scoped bindings resolve to per-scope
	// instances; closing the scope disposes instances implementing
	// io.Closer or Closable (in reverse creation order).
	Scope() ScopedResolver
}

// ScopedResolver is a child resolution scope with a bounded lifetime.
type ScopedResolver interface {
	Resolver
	// Close disposes scoped instances in reverse creation order.
	Close(ctx context.Context) error
}
