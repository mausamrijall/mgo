package main

// The whole auth surface on one page: session login (scs), JWT API
// (golang-jwt), argon2id passwords, ability-gated admin route — glued,
// not wrapped. Every route works identically on chi and stdmux; the
// feature tests run the full suite on both.

import (
	"context"
	"errors"
	"net/http"

	mgojwt "github.com/mgo-framework/mgo/adapters/auth-jwt"
	mgoargon2 "github.com/mgo-framework/mgo/adapters/hash-argon2"
	mgochi "github.com/mgo-framework/mgo/adapters/router-chi"
	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	mgoscs "github.com/mgo-framework/mgo/adapters/session-scs"
	authc "github.com/mgo-framework/mgo/contracts/auth"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/framework/auth"
	"github.com/mgo-framework/mgo/framework/middleware"
	"github.com/mgo-framework/mgo/framework/web"
)

// user is the in-memory account record (a real app keeps these in the
// store from Phase 4).
type user struct {
	email string
	hash  string // argon2id PHC string
	role  string
}

type app struct {
	users    map[string]user // email → user
	sessions *mgoscs.Sessions
	jwt      *mgojwt.Guard
	gate     *auth.Gate
}

func newApp(secret []byte) (*app, error) {
	users := map[string]user{}
	for _, u := range []struct{ email, password, role string }{
		{"admin@example.com", "admin123", "admin"},
		{"user@example.com", "user123", "member"},
	} {
		h, err := mgoargon2.Hash(u.password)
		if err != nil {
			return nil, err
		}
		users[u.email] = user{email: u.email, hash: h, role: u.role}
	}

	a := &app{
		users:    users,
		sessions: mgoscs.New(),
		jwt:      mgojwt.New(mgojwt.Config{Secret: secret, Issuer: "authdemo"}),
	}
	a.gate = auth.NewGate().Define("admin", func(ctx context.Context, id authc.Identity) error {
		if u, ok := a.users[id.Subject()]; ok && u.role == "admin" {
			return nil
		}
		return errors.New("admin role required")
	})
	return a, nil
}

// checkPassword returns the user when the credentials are valid.
func (a *app) checkPassword(email, password string) (user, bool) {
	u, ok := a.users[email]
	if !ok {
		// Burn comparable time so unknown emails aren't distinguishable.
		mgoargon2.Verify(password, "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		return user{}, false
	}
	ok, err := mgoargon2.Verify(password, u.hash)
	return u, err == nil && ok
}

// handler assembles the full HTTP surface on the chosen router.
func (a *app) handler(router string) http.Handler {
	// Route-level chains (stdmux has no groups, so both routers get the
	// same explicit per-route composition — portable by construction).
	web_ := func(h http.Handler) http.Handler { // session + CSRF
		return middleware.Chain(h, a.sessions.Middleware(), middleware.CSRF())
	}
	webAuthed := func(h http.Handler) http.Handler { // + identity required
		return web_(middleware.Chain(h, auth.Authenticate(a.sessions.Guard()), auth.Require()))
	}
	api := func(h http.Handler) http.Handler { // bearer only, no cookies
		return middleware.Chain(h, auth.Authenticate(a.jwt), auth.Require())
	}
	adminChain := func(h http.Handler) http.Handler { // session OR jwt, gated
		return middleware.Chain(h,
			a.sessions.Middleware(),
			auth.Authenticate(a.sessions.Guard(), a.jwt),
			auth.RequireAbility(a.gate, "admin"))
	}

	routes := map[string]http.Handler{
		"POST /login":  web_(http.HandlerFunc(a.loginHandler)),
		"POST /logout": webAuthed(http.HandlerFunc(a.logoutHandler)),
		"GET /me":      webAuthed(http.HandlerFunc(meHandler)),
		"POST /token":  http.HandlerFunc(a.tokenHandler),
		"GET /api/data": api(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := authc.FromContext(r.Context())
			web.JSON(w, http.StatusOK, map[string]string{"data": "secret payload", "sub": id.Subject()})
		})),
		"GET /admin": adminChain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			web.JSON(w, http.StatusOK, map[string]string{"admin": "true"})
		})),
	}

	global := []routerc.Middleware{middleware.RequestID(), middleware.Recover(nil), middleware.SecureHeaders()}

	switch router {
	case "stdmux":
		r := stdmux.New()
		r.Use(global...)
		for pattern, h := range routes {
			r.Handle(pattern, h)
		}
		return r
	default: // chi
		r := mgochi.New()
		r.Use(global...)
		for pattern, h := range routes {
			method, path, _ := cutPattern(pattern)
			r.Method(method, path, h)
		}
		return r
	}
}

func cutPattern(p string) (method, path string, ok bool) {
	for i := range p {
		if p[i] == ' ' {
			return p[:i], p[i+1:], true
		}
	}
	return "", p, false
}

// ---- handlers ----

type creds struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *app) loginHandler(w http.ResponseWriter, r *http.Request) {
	var c creds
	if err := web.Bind(r, &c); err != nil {
		web.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	u, ok := a.checkPassword(c.Email, c.Password)
	if !ok {
		web.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := a.sessions.Login(r.Context(), authc.Subject(u.email)); err != nil {
		web.Error(w, http.StatusInternalServerError, "session error")
		return
	}
	web.NoContent(w, http.StatusNoContent)
}

func (a *app) logoutHandler(w http.ResponseWriter, r *http.Request) {
	a.sessions.Logout(r.Context())
	web.NoContent(w, http.StatusNoContent)
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := authc.FromContext(r.Context())
	web.JSON(w, http.StatusOK, map[string]string{"subject": id.Subject()})
}

func (a *app) tokenHandler(w http.ResponseWriter, r *http.Request) {
	var c creds
	if err := web.Bind(r, &c); err != nil {
		web.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	u, ok := a.checkPassword(c.Email, c.Password)
	if !ok {
		web.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := a.jwt.Issue(u.email, map[string]any{"role": u.role})
	if err != nil {
		web.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"token": token})
}
