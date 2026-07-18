// Optional container capabilities, discovered by type assertion — the same
// pattern as router.RouteLister. A container implementation without them
// still satisfies Container; you only lose the corresponding feature.
package container

// ContextualBinder supports contextual bindings: when resolving consumer's
// constructor parameters, key resolves through this constructor instead of
// the default binding. Laravel's when()->needs()->give(), Go-shaped.
type ContextualBinder interface {
	// BindFor registers constructor for key, applied only while building
	// consumer. Both consumer and key are typed nil pointers like (*T)(nil).
	BindFor(consumer any, key any, constructor any, lifetime Lifetime) error
}

// FuncBinder supports typed resolver-function constructors that bypass
// reflection entirely: the function receives a Resolver and pulls its own
// dependencies. This is the shape `mgo --di=codegen` emits — hand-written
// or generated, it is the zero-reflection fast path.
type FuncBinder interface {
	BindFunc(key any, fn func(Resolver) (any, error), lifetime Lifetime) error
}

// Deferrer supports deferred registration: register runs the first time
// any of keys is resolved, not at boot. Used by the kernel for
// app.Deferrable providers; single-flight per group.
type Deferrer interface {
	Defer(register func() error, keys ...any) error
}
