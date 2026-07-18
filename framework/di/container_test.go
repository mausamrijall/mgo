package di_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mgo-framework/mgo/contracts/container"
	"github.com/mgo-framework/mgo/framework/di"
)

// ---- fixtures ----

type Repo interface{ Find(id int) string }

type sqlRepo struct{ prefix string }

func (r *sqlRepo) Find(id int) string { return r.prefix }

func newSQLRepo() *sqlRepo { return &sqlRepo{prefix: "sql"} }

type Service struct {
	Repo Repo
}

func newService(r Repo) *Service { return &Service{Repo: r} }

type closableDep struct {
	closed *atomic.Int32
}

func (c *closableDep) Close(context.Context) error { c.closed.Add(1); return nil }

// ---- tests ----

func TestTransientProducesNewInstances(t *testing.T) {
	c := di.New()
	if err := di.Bind[*sqlRepo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	a := di.MustMake[*sqlRepo](c)
	b := di.MustMake[*sqlRepo](c)
	if a == b {
		t.Fatal("transient resolves must differ")
	}
}

func TestSingletonMemoizesAndIsConcurrencySafe(t *testing.T) {
	c := di.New()
	var builds atomic.Int32
	if err := di.Singleton[Repo](c, func() Repo { builds.Add(1); return newSQLRepo() }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]Repo, 50)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = di.MustMake[Repo](c)
		}(i)
	}
	wg.Wait()
	if builds.Load() != 1 {
		t.Fatalf("singleton built %d times", builds.Load())
	}
	for _, r := range results {
		if r != results[0] {
			t.Fatal("singleton instances differ")
		}
	}
}

func TestInterfaceBindingAndConstructorInjection(t *testing.T) {
	c := di.New()
	if err := di.Singleton[Repo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	if err := di.Bind[*Service](c, newService); err != nil {
		t.Fatal(err)
	}
	svc := di.MustMake[*Service](c)
	if svc.Repo.Find(1) != "sql" {
		t.Fatal("dependency not injected")
	}
}

func TestMissingInterfaceBindingDiagnostic(t *testing.T) {
	c := di.New()
	if err := di.Bind[*Service](c, newService); err != nil {
		t.Fatal(err)
	}
	_, err := di.Make[*Service](c)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"no binding", "di_test.Repo", "interfaces require an explicit binding", "→"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostic %q missing %q", err.Error(), want)
		}
	}
}

func TestCycleDetection(t *testing.T) {
	type A struct{}
	type B struct{}
	c := di.New()
	if err := di.Bind[*A](c, func(*B) *A { return &A{} }); err != nil {
		t.Fatal(err)
	}
	if err := di.Bind[*B](c, func(*A) *B { return &B{} }); err != nil {
		t.Fatal(err)
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestValidateReportsMissingBinding(t *testing.T) {
	c := di.New()
	if err := di.Bind[*Service](c, newService); err != nil {
		t.Fatal(err)
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "no binding for di_test.Repo") {
		t.Fatalf("want missing-binding error, got %v", err)
	}
}

func TestScopedLifetime(t *testing.T) {
	c := di.New()
	if err := di.Scoped[*sqlRepo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}

	// Root resolution of scoped binding must fail.
	if _, err := di.Make[*sqlRepo](c); err == nil || !strings.Contains(err.Error(), "Scoped") {
		t.Fatalf("want scoped-from-root error, got %v", err)
	}

	s1, s2 := c.Scope(), c.Scope()
	a1 := mustScope[*sqlRepo](t, s1)
	a2 := mustScope[*sqlRepo](t, s1)
	b1 := mustScope[*sqlRepo](t, s2)
	if a1 != a2 {
		t.Fatal("same scope must memoize")
	}
	if a1 == b1 {
		t.Fatal("different scopes must differ")
	}
}

func TestScopeCloseDisposesInReverseOrder(t *testing.T) {
	c := di.New()
	var closed atomic.Int32
	if err := di.Scoped[*closableDep](c, func() *closableDep { return &closableDep{closed: &closed} }); err != nil {
		t.Fatal(err)
	}
	s := c.Scope()
	_ = mustScope[*closableDep](t, s)
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 1 {
		t.Fatalf("closed %d times", closed.Load())
	}
	// Resolving after close fails.
	if _, err := s.Resolve((**closableDep)(nil)); err == nil {
		t.Fatal("resolve after close must fail")
	}
}

func TestLifetimeViolationSingletonOverScoped(t *testing.T) {
	c := di.New()
	if err := di.Scoped[*sqlRepo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	if err := di.Singleton[*Service](c, func(r *sqlRepo) *Service { return &Service{Repo: r} }); err != nil {
		t.Fatal(err)
	}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "lifetime violation") {
		t.Fatalf("want lifetime violation, got %v", err)
	}
}

func TestConstructorErrorPropagates(t *testing.T) {
	c := di.New()
	boom := errors.New("boom")
	if err := di.Singleton[Repo](c, func() (Repo, error) { return nil, boom }); err != nil {
		t.Fatal(err)
	}
	_, err := di.Make[Repo](c)
	if !errors.Is(err, boom) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestCallInjectsParamsAndSurfacesError(t *testing.T) {
	c := di.New()
	if err := di.Singleton[Repo](c, newSQLRepo); err != nil {
		t.Fatal(err)
	}
	out, err := c.Call(func(r Repo) (string, error) { return r.Find(1), nil })
	if err != nil || out[0].(string) != "sql" {
		t.Fatalf("call failed: %v %v", out, err)
	}
	boom := errors.New("boom")
	_, err = c.Call(func(Repo) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestInstanceRegistration(t *testing.T) {
	c := di.New()
	r := newSQLRepo()
	if err := di.Instance[Repo](c, r); err != nil {
		t.Fatal(err)
	}
	if got := di.MustMake[Repo](c); got != Repo(r) {
		t.Fatal("instance mismatch")
	}
}

func TestAutoResolutionOfConcreteStruct(t *testing.T) {
	type Plain struct{ N int }
	c := di.New()
	v, err := di.Make[*Plain](c)
	if err != nil || v == nil {
		t.Fatalf("auto-resolve failed: %v", err)
	}
}

func TestSealedAfterValidate(t *testing.T) {
	c := di.New()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := di.Bind[*sqlRepo](c, newSQLRepo); err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("want sealed error, got %v", err)
	}
}

func mustScope[T any](t *testing.T, s container.ScopedResolver) T {
	t.Helper()
	v, err := s.Resolve((*T)(nil))
	if err != nil {
		t.Fatal(err)
	}
	return v.(T)
}
