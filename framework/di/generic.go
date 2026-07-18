package di

import (
	"fmt"

	"github.com/mgo-framework/mgo/contracts/container"
)

func capabilityErr(name string) error {
	return fmt.Errorf("di: this container does not implement the %s capability", name)
}

// Generic, type-safe front door to the container (doc 02 §6.1). These are the
// primary user API; the reflective methods on Container are the mechanism.

// Bind registers constructor for interface-or-concrete type T as Transient.
func Bind[T any](c container.Container, constructor any) error {
	return c.Bind((*T)(nil), constructor, container.Transient)
}

// Singleton registers constructor for T with Singleton lifetime.
func Singleton[T any](c container.Container, constructor any) error {
	return c.Bind((*T)(nil), constructor, container.Singleton)
}

// Scoped registers constructor for T with Scoped lifetime.
func Scoped[T any](c container.Container, constructor any) error {
	return c.Bind((*T)(nil), constructor, container.Scoped)
}

// Instance registers an existing value for T.
func Instance[T any](c container.Container, value T) error {
	return c.Instance((*T)(nil), value)
}

// BindFor registers a contextual binding: while constructing Consumer,
// parameters of type T resolve through this constructor instead of the
// default binding. Requires the container.ContextualBinder capability.
func BindFor[Consumer any, T any](c container.Container, constructor any, lifetime container.Lifetime) error {
	cb, ok := c.(container.ContextualBinder)
	if !ok {
		return capabilityErr("ContextualBinder")
	}
	return cb.BindFor((*Consumer)(nil), (*T)(nil), constructor, lifetime)
}

// The *Func variants register typed resolver-function constructors that
// bypass reflection entirely — the shape `mgo --di=codegen` emits:
//
//	di.SingletonFunc[*UserService](c, func(r container.Resolver) (*UserService, error) {
//	    repo, err := di.Make[UserRepo](r)
//	    if err != nil { return nil, err }
//	    return NewUserService(repo), nil
//	})

// SingletonFunc registers fn for T with Singleton lifetime, no reflection.
func SingletonFunc[T any](c container.Container, fn func(container.Resolver) (T, error)) error {
	return bindFunc[T](c, fn, container.Singleton)
}

// ScopedFunc registers fn for T with Scoped lifetime, no reflection.
func ScopedFunc[T any](c container.Container, fn func(container.Resolver) (T, error)) error {
	return bindFunc[T](c, fn, container.Scoped)
}

// TransientFunc registers fn for T with Transient lifetime, no reflection.
func TransientFunc[T any](c container.Container, fn func(container.Resolver) (T, error)) error {
	return bindFunc[T](c, fn, container.Transient)
}

func bindFunc[T any](c container.Container, fn func(container.Resolver) (T, error), lt container.Lifetime) error {
	fb, ok := c.(container.FuncBinder)
	if !ok {
		return capabilityErr("FuncBinder")
	}
	return fb.BindFunc((*T)(nil), func(r container.Resolver) (any, error) { return fn(r) }, lt)
}

// Defer registers fn to run the first time any of keys is resolved.
// Requires the container.Deferrer capability; the kernel uses this for
// app.Deferrable providers.
func Defer(c container.Container, register func() error, keys ...any) error {
	d, ok := c.(container.Deferrer)
	if !ok {
		return capabilityErr("Deferrer")
	}
	return d.Defer(register, keys...)
}

// Make resolves T from any resolver (root container or scope).
func Make[T any](r container.Resolver) (T, error) {
	var zero T
	v, err := r.Resolve((*T)(nil))
	if err != nil {
		return zero, err
	}
	return v.(T), nil
}

// MustMake resolves T or panics. Intended for boot-time composition code
// where a failure is a programming error already caught by Validate.
func MustMake[T any](r container.Resolver) T {
	v, err := Make[T](r)
	if err != nil {
		panic(err)
	}
	return v
}
