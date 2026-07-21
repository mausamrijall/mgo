package benchmarks

// Kernel budget gates — the constitution as failing tests:
//
//	1. contracts/go.mod has ZERO require directives, forever.
//	2. framework/go.mod requires ONLY contracts.
//	3. The kernel (mgo, di, conf, httpserver) stays under its LOC budget.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func parseMod(t *testing.T, path string) *modfile.File {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := modfile.Parse(path, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestContractsHaveZeroDependencies(t *testing.T) {
	f := parseMod(t, "../contracts/go.mod")
	if len(f.Require) != 0 {
		t.Fatalf("contracts/go.mod has %d requires — the zero-dep invariant is forever", len(f.Require))
	}
}

func TestFrameworkRequiresOnlyContracts(t *testing.T) {
	f := parseMod(t, "../framework/go.mod")
	for _, r := range f.Require {
		if r.Mod.Path != "github.com/mgo-framework/mgo/contracts" {
			t.Fatalf("framework/go.mod requires %s — only contracts is allowed", r.Mod.Path)
		}
	}
}

// kernelBudget is the doc-02 kernel LOC budget (non-test Go lines).
const kernelBudget = 8000

func TestKernelStaysWithinLOCBudget(t *testing.T) {
	kernel := []string{"mgo", "di", "conf", "httpserver"}
	total := 0
	for _, pkg := range kernel {
		dir := filepath.Join("..", "framework", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			total += strings.Count(string(raw), "\n")
		}
	}
	t.Logf("kernel LOC: %d / %d budget", total, kernelBudget)
	if total > kernelBudget {
		t.Fatalf("kernel is %d lines — over the %d budget; trim or move to adapters", total, kernelBudget)
	}
}
