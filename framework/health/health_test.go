package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	healthc "github.com/mgo-framework/mgo/contracts/health"
	"github.com/mgo-framework/mgo/framework/health"
)

func ok(ctx context.Context) error   { return nil }
func bad(ctx context.Context) error  { return errors.New("connection refused") }
func slow(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func TestReadyAllHealthy(t *testing.T) {
	r := health.New(time.Second).
		Add("db", healthc.CheckerFunc(ok)).
		Add("cache", healthc.CheckerFunc(ok))
	rec := httptest.NewRecorder()
	r.ReadyHandler()(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Ready  bool
		Checks []health.Result
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Ready || len(body.Checks) != 2 {
		t.Fatalf("body = %+v", body)
	}
	// Sorted by name: cache before db.
	if body.Checks[0].Name != "cache" || body.Checks[1].Name != "db" {
		t.Fatalf("order = %v", body.Checks)
	}
}

func TestReadyFailingDependency(t *testing.T) {
	r := health.New(time.Second).
		Add("db", healthc.CheckerFunc(ok)).
		Add("broker", healthc.CheckerFunc(bad))
	rec := httptest.NewRecorder()
	r.ReadyHandler()(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body struct{ Checks []health.Result }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Checks[0].Name != "broker" || body.Checks[0].Healthy || body.Checks[0].Error == "" {
		t.Fatalf("failing check detail = %+v", body.Checks[0])
	}
}

func TestSlowCheckerIsBoundedByTimeout(t *testing.T) {
	r := health.New(50 * time.Millisecond).Add("stuck", healthc.CheckerFunc(slow))
	start := time.Now()
	results := r.Check(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("check took %s — timeout not applied", elapsed)
	}
	if results[0].Healthy {
		t.Fatal("timed-out checker reported healthy")
	}
}

func TestLiveIgnoresDependencies(t *testing.T) {
	rec := httptest.NewRecorder()
	health.LiveHandler()(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("live = %d, want 200 unconditionally", rec.Code)
	}
}
