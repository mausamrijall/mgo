package mgocors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mgocors "github.com/mgo-framework/mgo/adapters/middleware-cors"
)

func TestPreflight(t *testing.T) {
	mw := mgocors.New(mgocors.Config{
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{"GET", "POST", "DELETE"},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight must not reach the handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "DELETE" {
		t.Fatalf("allow-methods = %q", got)
	}
}

func TestDisallowedOrigin(t *testing.T) {
	mw := mgocors.New(mgocors.Config{AllowedOrigins: []string{"https://app.example.com"}})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin must not get allow-origin header")
	}
}
