package di

import (
	"github.com/mgo-framework/mgo/contracts/container"
)

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
