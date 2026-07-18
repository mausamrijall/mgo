package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mgo-framework/mgo/contracts/container"
	"github.com/mgo-framework/mgo/framework/di"
	"github.com/mgo-framework/mgo/framework/middleware"
)

type perRequest struct {
	id     int
	closed *atomic.Int32
}

func (p *perRequest) Close(context.Context) error { p.closed.Add(1); return nil }

func TestScopeMiddlewarePerRequestIsolationAndDisposal(t *testing.T) {
	var closed atomic.Int32
	n := 0
	c := di.New()
	if err := di.ScopedFunc[*perRequest](c, func(container.Resolver) (*perRequest, error) {
		n++
		return &perRequest{id: n, closed: &closed}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	seen := map[int]bool{}
	h := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, ok := container.FromContext(r.Context())
		if !ok {
			t.Fatal("no scope in request context")
		}
		a := di.MustMake[*perRequest](scope)
		b := di.MustMake[*perRequest](scope)
		if a != b {
			t.Fatal("scope must memoize within one request")
		}
		seen[a.id] = true
	}), middleware.Scope(c))

	for range 3 {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct per-request instances, saw %d", len(seen))
	}
	if closed.Load() != 3 {
		t.Fatalf("expected 3 disposals, got %d", closed.Load())
	}
}
