// Package mgoscs adapts alexedwards/scs to MGO's auth contract: cookie
// sessions with secure defaults, Login/Logout helpers, and a Guard that
// authenticates the session cookie. The *scs.SessionManager is embedded —
// scs's native API (Put/Get/RememberMe/stores) stays fully available,
// including swapping the session store to redis/postgres via scs's own
// store implementations.
package mgoscs

import (
	"context"
	"net/http"

	"github.com/alexedwards/scs/v2"
	authc "github.com/mgo-framework/mgo/contracts/auth"
)

// identityKey is the session key holding the authenticated subject.
const identityKey = "mgo:subject"

// Sessions wraps an scs manager with auth glue.
type Sessions struct {
	*scs.SessionManager
}

// New builds a session manager with secure cookie defaults: HttpOnly,
// SameSite=Lax, Secure when the request is TLS (scs handles that
// per-request via Cookie.Secure=false + your TLS terminator; set
// s.Cookie.Secure = true behind HTTPS).
func New() *Sessions {
	m := scs.New()
	m.Cookie.HttpOnly = true
	m.Cookie.SameSite = http.SameSiteLaxMode
	return &Sessions{SessionManager: m}
}

// Middleware loads and saves the session around each request — scs's own
// LoadAndSave, which is already stdlib-shaped.
func (s *Sessions) Middleware() func(http.Handler) http.Handler {
	return s.LoadAndSave
}

// Login records the identity in the session, rotating the session token
// first (session-fixation defense).
func (s *Sessions) Login(ctx context.Context, id authc.Identity) error {
	if err := s.RenewToken(ctx); err != nil {
		return err
	}
	s.Put(ctx, identityKey, id.Subject())
	return nil
}

// Logout destroys the session.
func (s *Sessions) Logout(ctx context.Context) error {
	return s.Destroy(ctx)
}

// Guard returns a contracts/auth.Guard that authenticates the session.
// Use inside s.Middleware (the session must be loaded).
func (s *Sessions) Guard() authc.Guard {
	return authc.GuardFunc(func(r *http.Request) (authc.Identity, error) {
		sub := s.GetString(r.Context(), identityKey)
		if sub == "" {
			return nil, authc.ErrUnauthenticated
		}
		return authc.Subject(sub), nil
	})
}
