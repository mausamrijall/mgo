package di_test

// Phase 3 hardening tests: contextual bindings, deferred registration,
// typed func constructors, compiled-plan resolution, and the
// no-reflection/no-allocation hot-path guards.

import (
	"errors"
	"strings"
	"testing"

	"github.com/mgo-framework/mgo/contracts/container"
	"github.com/mgo-framework/mgo/framework/di"
)

type memRepo struct{ prefix string }

func (r *memRepo) Find(id int) string { return r.prefix }

func newMemRepo() *memRepo { return &memRepo{prefix: "mem"} }

type AuditService struct{ Repo Repo }

func newAuditService(r Repo) *AuditService { return &AuditService{Repo: r} }

func TestContextualBinding(t *testing.T) {
	c := di.New()
	if err := di.Singleton[Repo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	if err := di.Bind[*Service](c, newService); err != nil {
		t.Fatal(err)
	}
	if err := di.Bind[*AuditService](c, newAuditService); err != nil {
		t.Fatal(err)
	}
	// AuditService gets the in-memory repo; everyone else keeps sql.
	if err := di.BindFor[*AuditService, Repo](c, newMemRepo, container.Singleton); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	svc := di.MustMake[*Service](c)
	audit := di.MustMake[*AuditService](c)
	if svc.Repo.Find(1) != "sql" {
		t.Fatalf("default consumer got %q, want sql", svc.Repo.Find(1))
	}
	if audit.Repo.Find(1) != "mem" {
		t.Fatalf("contextual consumer got %q, want mem", audit.Repo.Find(1))
	}
}

func TestDeferredRegistrationRunsOnFirstResolve(t *testing.T) {
	c := di.New()
	registered := 0
	err := di.Defer(c, func() error {
		registered++
		return di.Singleton[Repo](c, newSQLRepo)
	}, (*Repo)(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil { // deferred key passes validation
		t.Fatal(err)
	}
	if registered != 0 {
		t.Fatal("deferred register ran before first resolve")
	}
	for range 3 {
		if r := di.MustMake[Repo](c); r.Find(1) != "sql" {
			t.Fatal("wrong instance")
		}
	}
	if registered != 1 {
		t.Fatalf("register ran %d times, want 1 (single-flight)", registered)
	}
}

func TestDeferredProviderMustDeliver(t *testing.T) {
	c := di.New()
	if err := di.Defer(c, func() error { return nil }, (*Repo)(nil)); err != nil {
		t.Fatal(err)
	}
	_, err := di.Make[Repo](c)
	if err == nil || !strings.Contains(err.Error(), "promised") {
		t.Fatalf("want promised-but-unbound error, got %v", err)
	}
}

func TestDeferredRegistrationErrorPropagates(t *testing.T) {
	c := di.New()
	boom := errors.New("boom")
	if err := di.Defer(c, func() error { return boom }, (*Repo)(nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := di.Make[Repo](c); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	// Error is sticky: the group never re-runs.
	if _, err := di.Make[Repo](c); !errors.Is(err, boom) {
		t.Fatalf("want sticky boom, got %v", err)
	}
}

func TestFuncBindingsBypassReflection(t *testing.T) {
	c := di.New()
	if err := di.SingletonFunc[Repo](c, func(r container.Resolver) (Repo, error) {
		return newSQLRepo(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := di.TransientFunc[*Service](c, func(r container.Resolver) (*Service, error) {
		repo, err := di.Make[Repo](r)
		if err != nil {
			return nil, err
		}
		return newService(repo), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	a, b := di.MustMake[*Service](c), di.MustMake[*Service](c)
	if a == b {
		t.Fatal("transient func returned same instance")
	}
	if a.Repo != b.Repo {
		t.Fatal("singleton func returned different instances")
	}
}

func TestScopedFuncPerScopeInstances(t *testing.T) {
	c := di.New()
	n := 0
	if err := di.ScopedFunc[*memRepo](c, func(container.Resolver) (*memRepo, error) {
		n++
		return newMemRepo(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	s1, s2 := c.Scope(), c.Scope()
	a1 := di.MustMake[*memRepo](s1)
	a2 := di.MustMake[*memRepo](s1)
	b1 := di.MustMake[*memRepo](s2)
	if a1 != a2 {
		t.Fatal("same scope must memoize")
	}
	if a1 == b1 {
		t.Fatal("different scopes must get different instances")
	}
	if n != 2 {
		t.Fatalf("constructor ran %d times, want 2", n)
	}
}

// The Phase 3 exit gate: after Validate, singleton and scoped hits must
// not allocate — proof that no reflection-driven work (path slices, arg
// boxing, type re-derivation) happens on the hot path.
func TestHotPathZeroAllocations(t *testing.T) {
	c := di.New()
	if err := di.Singleton[Repo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	if err := di.ScopedFunc[*memRepo](c, func(container.Resolver) (*memRepo, error) {
		return newMemRepo(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	di.MustMake[Repo](c) // warm the singleton
	scope := c.Scope()
	di.MustMake[*memRepo](scope) // warm the scope

	if n := testing.AllocsPerRun(1000, func() { di.MustMake[Repo](c) }); n != 0 {
		t.Fatalf("singleton hit allocates %.1f/op, want 0", n)
	}
	if n := testing.AllocsPerRun(1000, func() { di.MustMake[*memRepo](scope) }); n != 0 {
		t.Fatalf("scoped hit allocates %.1f/op, want 0", n)
	}
}

func BenchmarkSingletonHit(b *testing.B) {
	c := di.New()
	di.Singleton[Repo](c, newSQLRepo)
	c.Validate()
	di.MustMake[Repo](c)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		di.MustMake[Repo](c)
	}
}

func BenchmarkScopedHit(b *testing.B) {
	c := di.New()
	di.ScopedFunc[*memRepo](c, func(container.Resolver) (*memRepo, error) { return newMemRepo(), nil })
	c.Validate()
	s := c.Scope()
	di.MustMake[*memRepo](s)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		di.MustMake[*memRepo](s)
	}
}

func BenchmarkTransientReflectivePlan(b *testing.B) {
	c := di.New()
	di.Singleton[Repo](c, newSQLRepo)
	di.Bind[*Service](c, newService)
	c.Validate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		di.MustMake[*Service](c)
	}
}

func BenchmarkTransientFunc(b *testing.B) {
	c := di.New()
	di.Singleton[Repo](c, newSQLRepo)
	di.TransientFunc[*Service](c, func(r container.Resolver) (*Service, error) {
		repo, _ := di.Make[Repo](r)
		return newService(repo), nil
	})
	c.Validate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		di.MustMake[*Service](c)
	}
}
