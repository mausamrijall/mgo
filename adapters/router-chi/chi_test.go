package mgochi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	mgochi "github.com/mgo-framework/mgo/adapters/router-chi"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/contracts/routertest"
)

func TestConformance(t *testing.T) {
	routertest.Run(t, func() routerc.Router { return mgochi.New() })
}

// The contract is shaped so chi needs no adapting: a bare *chi.Mux passes
// conformance too.
func TestBareChiMuxConformance(t *testing.T) {
	routertest.Run(t, func() routerc.Router { return chi.NewRouter() })
}

func TestNativeAPIAndPathValue(t *testing.T) {
	r := mgochi.New()
	r.Get("/hello/{name}", func(w http.ResponseWriter, req *http.Request) {
		// chi populates both its own URLParam and Go 1.22 PathValue.
		w.Write([]byte(req.PathValue("name") + "," + chi.URLParam(req, "name")))
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/hello/go", nil))
	if rec.Body.String() != "go,go" {
		t.Fatalf("body = %q, want go,go", rec.Body.String())
	}
}

func TestRoutesMetadata(t *testing.T) {
	r := mgochi.New()
	r.Get("/users/{id}", func(http.ResponseWriter, *http.Request) {})
	r.Post("/users", func(http.ResponseWriter, *http.Request) {})

	routes := r.Routes()
	seen := map[string]bool{}
	for _, rt := range routes {
		seen[rt.Method+" "+rt.Pattern] = true
	}
	if !seen["GET /users/{id}"] || !seen["POST /users"] {
		t.Fatalf("routes = %+v", routes)
	}
}

// Benchmarks: the adapter must stay within 5% of raw chi. Since Router
// embeds *chi.Mux and adds nothing to the serve path, the two should be
// indistinguishable.
// nopWriter avoids recorder allocations so the router's own serve path
// dominates the measurement.
type nopWriter struct{ h http.Header }

func (w nopWriter) Header() http.Header       { return w.h }
func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriter) WriteHeader(int)             {}

func benchServe(b *testing.B, h http.Handler) {
	req := httptest.NewRequest("GET", "/users/42", nil)
	w := nopWriter{h: make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

func BenchmarkRawChi(b *testing.B) {
	r := chi.NewRouter()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(200) })
	benchServe(b, r)
}

func BenchmarkAdapter(b *testing.B) {
	r := mgochi.New()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(200) })
	benchServe(b, r)
}
