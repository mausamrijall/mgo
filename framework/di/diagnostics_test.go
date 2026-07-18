package di_test

// Snapshot tests for container diagnostics: the exact wording of graph
// errors is part of MGO's DX surface, so it is pinned with golden files.
// Regenerate deliberately with: go test ./framework/di -run Diagnostics -update

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgo-framework/mgo/contracts/container"
	"github.com/mgo-framework/mgo/framework/di"
)

var update = flag.Bool("update", false, "rewrite golden files")

type cycleA struct{}
type cycleB struct{}

func newCycleA(*cycleB) *cycleA { return &cycleA{} }
func newCycleB(*cycleA) *cycleB { return &cycleB{} }

func TestDiagnosticsSnapshots(t *testing.T) {
	cases := []struct {
		name string
		err  func(t *testing.T) error
	}{
		{"missing_binding", func(t *testing.T) error {
			c := di.New()
			must(t, di.Bind[*Service](c, newService))
			return c.Validate()
		}},
		{"cycle", func(t *testing.T) error {
			c := di.New()
			must(t, di.Bind[*cycleA](c, newCycleA))
			must(t, di.Bind[*cycleB](c, newCycleB))
			return c.Validate()
		}},
		{"lifetime_violation", func(t *testing.T) error {
			c := di.New()
			must(t, di.Scoped[Repo](c, newSQLRepo))
			must(t, di.Singleton[*Service](c, newService))
			return c.Validate()
		}},
		{"scoped_from_root", func(t *testing.T) error {
			c := di.New()
			must(t, di.Scoped[Repo](c, newSQLRepo))
			must(t, c.Validate())
			_, err := di.Make[Repo](c)
			return err
		}},
		{"sealed", func(t *testing.T) error {
			c := di.New()
			must(t, c.Validate())
			return di.Bind[Repo](c, newSQLRepo)
		}},
		{"deferred_unfulfilled", func(t *testing.T) error {
			c := di.New()
			must(t, di.Defer(c, func() error { return nil }, (*Repo)(nil)))
			must(t, c.Validate())
			_, err := di.Make[Repo](c)
			return err
		}},
		{"resolve_unknown_interface", func(t *testing.T) error {
			c := di.New()
			must(t, c.Validate())
			_, err := di.Make[Repo](c)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.err(t)
			if err == nil {
				t.Fatal("expected a diagnostic error")
			}
			golden := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(err.Error()+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, rerr := os.ReadFile(golden)
			if rerr != nil {
				t.Fatalf("missing golden file (run with -update): %v", rerr)
			}
			if got := err.Error() + "\n"; got != string(want) {
				t.Fatalf("diagnostic drifted from snapshot.\ngot:  %q\nwant: %q", got, string(want))
			}
		})
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Keep the capability pattern honest: a bare contract value without the
// optional interfaces gets a clear error, not a panic.
func TestCapabilityErrors(t *testing.T) {
	var c container.Container = minimalContainer{}
	if err := di.BindFor[*Service, Repo](c, newSQLRepo, container.Singleton); err == nil {
		t.Fatal("want ContextualBinder capability error")
	}
	if err := di.SingletonFunc[Repo](c, nil); err == nil {
		t.Fatal("want FuncBinder capability error")
	}
	if err := di.Defer(c, func() error { return nil }, (*Repo)(nil)); err == nil {
		t.Fatal("want Deferrer capability error")
	}
}

// minimalContainer implements only the base contract.
type minimalContainer struct{ container.Container }
