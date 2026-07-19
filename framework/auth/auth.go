// Package auth is MGO's first-party authentication and authorization
// glue: guard middleware and a small ability Gate. Both operate on
// stdlib-shaped middleware and contracts/auth — no framework Ctx, no
// session or token opinions (those are adapters).
package auth

import (
	"context"
	"fmt"
	"net/http"

	authc "github.com/mgo-framework/mgo/contracts/auth"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/framework/web"
)

// Authenticate tries each guard in order; the first success stores the
// identity in the request context. Anonymous requests pass through —
// combine with Require (or RequireAbility) on routes that need a user.
func Authenticate(guards ...authc.Guard) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, g := range guards {
				if id, err := g.Authenticate(r); err == nil && id != nil {
					r = r.WithContext(authc.NewContext(r.Context(), id))
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Require rejects anonymous requests with 401. Place after Authenticate.
func Require() routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authc.FromContext(r.Context()); !ok {
				web.Error(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Ability decides whether an identity may do something. Return nil to
// allow; the error explains the denial (surfaced only in logs, not to
// the client).
type Ability func(ctx context.Context, id authc.Identity) error

// Gate is a registry of named abilities — MGO's minimal first-party
// authorization. For rule engines, use a casbin adapter instead; the
// middleware shape is the same.
type Gate struct {
	abilities map[string]Ability
}

// NewGate returns an empty Gate.
func NewGate() *Gate { return &Gate{abilities: map[string]Ability{}} }

// Define registers an ability by name (last write wins).
func (g *Gate) Define(name string, fn Ability) *Gate {
	g.abilities[name] = fn
	return g
}

// Allows checks the named ability for the identity in ctx.
func (g *Gate) Allows(ctx context.Context, name string) error {
	id, ok := authc.FromContext(ctx)
	if !ok {
		return authc.ErrUnauthenticated
	}
	fn, ok := g.abilities[name]
	if !ok {
		return fmt.Errorf("auth: undefined ability %q", name)
	}
	return fn(ctx, id)
}

// RequireAbility rejects requests whose identity fails the named ability:
// 401 when anonymous, 403 when denied. Place after Authenticate.
func RequireAbility(g *Gate, name string) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := g.Allows(r.Context(), name); err != nil {
				if err == authc.ErrUnauthenticated {
					web.Error(w, http.StatusUnauthorized, "unauthenticated")
					return
				}
				web.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
