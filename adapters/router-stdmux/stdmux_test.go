package stdmux_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/contracts/routertest"
)

func TestConformance(t *testing.T) {
	routertest.Run(t, func() routerc.Router { return stdmux.New() })
}

func TestNativeAPIAndPathValue(t *testing.T) {
	r := stdmux.New()
	r.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(req.PathValue("name")))
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/hello/go", nil))
	if rec.Body.String() != "go" {
		t.Fatalf("body = %q, want go", rec.Body.String())
	}
}

func TestRoutesMetadata(t *testing.T) {
	r := stdmux.New()
	r.HandleFunc("GET /users/{id}", func(http.ResponseWriter, *http.Request) {})
	r.Handle("POST /users", http.NotFoundHandler())
	r.Mount("/api", http.NotFoundHandler())

	routes := r.Routes()
	if len(routes) != 3 {
		t.Fatalf("got %d routes: %+v", len(routes), routes)
	}
	if routes[0] != (routerc.Route{Method: "GET", Pattern: "/users/{id}"}) {
		t.Fatalf("route[0] = %+v", routes[0])
	}
	if routes[1] != (routerc.Route{Method: "POST", Pattern: "/users"}) {
		t.Fatalf("route[1] = %+v", routes[1])
	}
	if routes[2].Method != "" || routes[2].Pattern != "/api/" {
		t.Fatalf("route[2] = %+v", routes[2])
	}
}

func TestUseAfterServePanics(t *testing.T) {
	r := stdmux.New()
	r.HandleFunc("GET /", func(http.ResponseWriter, *http.Request) {})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic from Use after serving")
		}
	}()
	r.Use(func(h http.Handler) http.Handler { return h })
}
