// Package routertest is the conformance suite for contracts/router
// implementations. Every router adapter runs Run in its tests; a
// third-party adapter that passes is contract-compliant.
//
// v1 asserts the three contract semantics: Mount reachability with
// unmodified r.URL.Path, Use ordering (first registered = outermost),
// and middleware applying to mounted handlers.
package routertest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Run executes the conformance suite. newRouter must return a fresh,
// empty router per call; Use is always exercised before any Mount, as the
// contract requires.
func Run(t *testing.T, newRouter func() routerc.Router) {
	t.Helper()

	t.Run("mount reachable at subpaths with full path preserved", func(t *testing.T) {
		r := newRouter()
		r.Mount("/api", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, req.URL.Path)
		}))
		body, status := get(t, r, "/api/users/42")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if body != "/api/users/42" {
			t.Fatalf("handler saw path %q, want unmodified /api/users/42", body)
		}
	})

	t.Run("use ordering first-is-outermost", func(t *testing.T) {
		r := newRouter()
		r.Use(appendMW("a"), appendMW("b"))
		r.Use(appendMW("c"))
		r.Mount("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Add("X-Trace", "handler")
		}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		got := fmt.Sprint(rec.Header().Values("X-Trace"))
		want := fmt.Sprint([]string{"a", "b", "c", "handler"})
		if got != want {
			t.Fatalf("middleware order = %v, want %v", got, want)
		}
	})

	t.Run("middleware wraps mounted handlers", func(t *testing.T) {
		r := newRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Wrapped", "yes")
				next.ServeHTTP(w, req)
			})
		})
		r.Mount("/svc", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/svc/ping", nil))
		if rec.Header().Get("X-Wrapped") != "yes" {
			t.Fatal("middleware did not wrap mounted handler")
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	})
}

func appendMW(tag string) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Trace", tag)
			next.ServeHTTP(w, r)
		})
	}
}

func get(t *testing.T, h http.Handler, path string) (string, int) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Body.String(), rec.Code
}
