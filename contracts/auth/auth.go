// Package auth defines MGO's authentication integration points. Like every
// MGO contract it captures glue only: what a guard produces (an Identity)
// and how it travels (the request context). Token formats, session
// storage, password hashing — all adapter territory, all deletable.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// ErrUnauthenticated is returned by guards when the request carries no
// valid credentials. Middleware maps it to 401.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

// Identity is an authenticated principal. Adapters may return richer
// types (JWT claims, user records); glue needs only a stable subject.
type Identity interface {
	Subject() string
}

// Subject is the minimal Identity — adapters that only know an id use it.
type Subject string

// Subject implements Identity.
func (s Subject) Subject() string { return string(s) }

// Guard authenticates an HTTP request. Implementations: JWT bearer,
// session cookie, API key, OIDC — anything that can say who this is.
type Guard interface {
	// Authenticate returns the request's identity, ErrUnauthenticated if
	// no credentials are present or they are invalid.
	Authenticate(r *http.Request) (Identity, error)
}

// GuardFunc adapts a function to Guard.
type GuardFunc func(r *http.Request) (Identity, error)

// Authenticate implements Guard.
func (f GuardFunc) Authenticate(r *http.Request) (Identity, error) { return f(r) }

type ctxKey struct{}

// NewContext returns ctx carrying the identity.
func NewContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the identity stored in ctx, if any.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
