package container

import "context"

type ctxKey struct{}

// NewContext returns ctx carrying the scope. The kernel's scope middleware
// stores each request's scope this way; handlers and services retrieve it
// with FromContext.
func NewContext(ctx context.Context, s ScopedResolver) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext returns the scope stored in ctx, if any.
func FromContext(ctx context.Context) (ScopedResolver, bool) {
	s, ok := ctx.Value(ctxKey{}).(ScopedResolver)
	return s, ok
}
