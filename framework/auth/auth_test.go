package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authc "github.com/mgo-framework/mgo/contracts/auth"
	"github.com/mgo-framework/mgo/framework/auth"
	"github.com/mgo-framework/mgo/framework/middleware"
)

func guardFor(sub string) authc.Guard {
	return authc.GuardFunc(func(r *http.Request) (authc.Identity, error) {
		if r.Header.Get("X-Token") == sub {
			return authc.Subject(sub), nil
		}
		return nil, authc.ErrUnauthenticated
	})
}

func whoami(w http.ResponseWriter, r *http.Request) {
	if id, ok := authc.FromContext(r.Context()); ok {
		w.Write([]byte(id.Subject()))
		return
	}
	w.Write([]byte("anonymous"))
}

func serve(h http.Handler, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/", nil)
	if header != "" {
		req.Header.Set("X-Token", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticateFirstGuardWins(t *testing.T) {
	h := middleware.Chain(http.HandlerFunc(whoami), auth.Authenticate(guardFor("alice"), guardFor("bob")))
	if got := serve(h, "bob").Body.String(); got != "bob" {
		t.Fatalf("second guard: got %q", got)
	}
	if got := serve(h, "alice").Body.String(); got != "alice" {
		t.Fatalf("first guard: got %q", got)
	}
	if got := serve(h, "").Body.String(); got != "anonymous" {
		t.Fatalf("anonymous passes through Authenticate: got %q", got)
	}
}

func TestRequire(t *testing.T) {
	h := middleware.Chain(http.HandlerFunc(whoami), auth.Authenticate(guardFor("alice")), auth.Require())
	if rec := serve(h, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rec.Code)
	}
	if rec := serve(h, "alice"); rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", rec.Code)
	}
}

func TestGateAndRequireAbility(t *testing.T) {
	gate := auth.NewGate().Define("admin", func(ctx context.Context, id authc.Identity) error {
		if id.Subject() != "alice" {
			return errors.New("not an admin")
		}
		return nil
	})

	h := middleware.Chain(http.HandlerFunc(whoami),
		auth.Authenticate(guardFor("alice"), guardFor("bob")),
		auth.RequireAbility(gate, "admin"))

	if rec := serve(h, "alice"); rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200", rec.Code)
	}
	if rec := serve(h, "bob"); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", rec.Code)
	}
	if rec := serve(h, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rec.Code)
	}
}

func TestGateUndefinedAbilityDenies(t *testing.T) {
	gate := auth.NewGate()
	ctx := authc.NewContext(context.Background(), authc.Subject("x"))
	if err := gate.Allows(ctx, "nope"); err == nil {
		t.Fatal("undefined ability must deny")
	}
}
