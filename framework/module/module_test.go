package module_test

// The Phase 9 exit: a Billing module living beside a CRM module, each
// fully namespaced — routes under /<name>/, config under
// modules.<name>, migrations aggregated — booted through the real app
// kernel, with cross-leak assertions.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	appc "github.com/mgo-framework/mgo/contracts/app"
	modulec "github.com/mgo-framework/mgo/contracts/module"
	ormc "github.com/mgo-framework/mgo/contracts/orm"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/mgo"
	"github.com/mgo-framework/mgo/framework/module"
)

// testRouter is a minimal contracts/router.Router over ServeMux — also
// proof the contract is trivially implementable.
type testRouter struct {
	mux *http.ServeMux
	mw  []routerc.Middleware
}

func newTestRouter() *testRouter { return &testRouter{mux: http.NewServeMux()} }

func (r *testRouter) Use(mw ...routerc.Middleware) { r.mw = append(r.mw, mw...) }
func (r *testRouter) Mount(pattern string, h http.Handler) {
	r.mux.Handle(pattern, h)
	r.mux.Handle(pattern+"/", h)
}
func (r *testRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var h http.Handler = r.mux
	for i := len(r.mw) - 1; i >= 0; i-- {
		h = r.mw[i](h)
	}
	h.ServeHTTP(w, req)
}

// ---- the Billing module ----

type recordingMigrator struct{ runs *atomic.Int32 }

func (m recordingMigrator) Migrate(context.Context) error { m.runs.Add(1); return nil }

type billingModule struct {
	currency string // bound from modules.billing at boot
	seenPath string // path the handler observed (relative-mount proof)
	migrated atomic.Int32
}

func (b *billingModule) ModuleName() string          { return "billing" }
func (b *billingModule) Register(app appc.App) error { return nil }

func (b *billingModule) Boot(ctx context.Context, app appc.App) error {
	// The module reads ONLY its own config section.
	b.currency = modulec.Config(app.Config(), b).String("currency", "USD")
	return nil
}

func (b *billingModule) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /invoices", func(w http.ResponseWriter, r *http.Request) {
		b.seenPath = r.URL.Path
		fmt.Fprintf(w, `{"invoices":[],"currency":%q}`, b.currency)
	})
	return mux
}

func (b *billingModule) Migrations() ormc.Migrator { return recordingMigrator{runs: &b.migrated} }

var (
	_ modulec.HTTPModule     = (*billingModule)(nil)
	_ modulec.MigratorModule = (*billingModule)(nil)
)

// ---- the CRM module: routes only, no migrations ----

type crmModule struct{}

func (crmModule) ModuleName() string          { return "crm" }
func (crmModule) Register(app appc.App) error { return nil }
func (crmModule) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /contacts", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"contacts":[]}`)
	})
	return mux
}

// ---- the demo ----

func TestBillingModuleIsolation(t *testing.T) {
	cfg, err := conf.NewLoader().Overrides(map[string]any{
		"modules": map[string]any{
			"billing": map[string]any{"currency": "EUR"},
			"crm":     map[string]any{"currency": "IGNORED-BY-BILLING"},
		},
	}).Load()
	if err != nil {
		t.Fatal(err)
	}

	billing := &billingModule{}
	crm := crmModule{}

	// Modules boot through the real kernel as ordinary providers.
	app := mgo.New(mgo.WithConfig(cfg), mgo.WithProviders(module.Providers(billing, crm)...))
	if err := app.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}

	if problems := module.CheckAll(billing, crm); len(problems) != 0 {
		t.Fatalf("linter problems on clean modules: %v", problems)
	}

	r := newTestRouter()
	if err := module.MountAll(r, billing, crm); err != nil {
		t.Fatal(err)
	}
	if err := module.MigrateAll(context.Background(), billing, crm); err != nil {
		t.Fatal(err)
	}

	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}

	// Each module answers under its own prefix, with its own config.
	if code, body := get("/billing/invoices"); code != 200 || body != `{"invoices":[],"currency":"EUR"}` {
		t.Fatalf("/billing/invoices = %d %s", code, body)
	}
	if code, _ := get("/crm/contacts"); code != 200 {
		t.Fatalf("/crm/contacts = %d", code)
	}

	// Isolation: no cross-leaks, nothing at the root.
	for _, path := range []string{"/billing/contacts", "/crm/invoices", "/invoices", "/contacts"} {
		if code, _ := get(path); code != 404 {
			t.Fatalf("%s = %d, want 404 (namespace leak)", path, code)
		}
	}

	// The module saw a RELATIVE path — it is relocatable by construction.
	if billing.seenPath != "/invoices" {
		t.Fatalf("billing handler saw %q, want /invoices (prefix must be stripped)", billing.seenPath)
	}

	// Migrations aggregated: billing's ran exactly once.
	if billing.migrated.Load() != 1 {
		t.Fatalf("billing migrations ran %d times, want 1", billing.migrated.Load())
	}
}

// ---- linter verdicts ----

type badModule struct{ crmModule }

func (badModule) ModuleName() string   { return "Bad_Name!" }
func (badModule) Routes() http.Handler { return nil }

func TestCheckCatchesViolations(t *testing.T) {
	if p := module.Check(badModule{}); len(p) != 2 {
		t.Fatalf("bad module problems = %v, want name + nil-routes", p)
	}
	// Duplicate names across the set.
	if p := module.CheckAll(crmModule{}, crmModule{}); len(p) != 1 || p[0].Issue != "duplicate module name" {
		t.Fatalf("duplicate detection = %v", p)
	}
	// MountAll refuses duplicates too.
	if err := module.MountAll(newTestRouter(), crmModule{}, crmModule{}); err == nil {
		t.Fatal("MountAll accepted duplicate module names")
	}
}
