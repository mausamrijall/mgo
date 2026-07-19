package main

// End-to-end exit-criteria test: mgo new → compiling, tested app; swap
// round-trip keeps tests green; the hash guard protects user edits.
// Uses the minimal preset (stdmux, no db) so it runs offline.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mgo")
	c := exec.Command("go", "build", "-o", bin, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd) // cli/ → repo root
}

func mgo(t *testing.T, bin, dir string, args ...string) (string, error) {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "MGO_SRC="+repoRoot(t))
	out, err := c.CombinedOutput()
	return string(out), err
}

func TestNewSwapRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e in -short mode")
	}
	bin := buildCLI(t)
	work := t.TempDir()

	// Exit criterion: mgo new → compiling, tested app, fast.
	start := time.Now()
	out, err := mgo(t, bin, work, "new", "demo", "--preset", "minimal")
	if err != nil {
		t.Fatalf("mgo new: %v\n%s", err, out)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("mgo new took %s, exit criterion is <30s", elapsed)
	}
	app := filepath.Join(work, "demo")
	for _, f := range []string{"main.go", "router.go", "handlers.go", "main_test.go", "mgo.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(app, f)); err != nil {
			t.Fatalf("missing generated file %s", f)
		}
	}

	// Swap round-trip: stdmux → chi → stdmux, tests green at every step
	// (swap runs `go test ./...` itself and fails loudly otherwise).
	if out, err := mgo(t, bin, app, "swap", "router", "chi"); err != nil {
		t.Fatalf("swap to chi: %v\n%s", err, out)
	}
	if raw, _ := os.ReadFile(filepath.Join(app, "router.go")); !strings.Contains(string(raw), "router-chi") {
		t.Fatal("router.go not regenerated for chi")
	}
	if out, err := mgo(t, bin, app, "swap", "router", "stdmux"); err != nil {
		t.Fatalf("swap back to stdmux: %v\n%s", err, out)
	}

	// make generators drop compiling, tested code into the project.
	if out, err := mgo(t, bin, app, "make", "handler", "posts"); err != nil {
		t.Fatalf("make handler: %v\n%s", err, out)
	}
	if out, err := mgo(t, bin, app, "make", "provider", "cache"); err != nil {
		t.Fatalf("make provider: %v\n%s", err, out)
	}
	c := exec.Command("go", "test", "./...")
	c.Dir = app
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go test after make: %v\n%s", err, out)
	}

	// The hash guard: edit a generated file, swap must refuse.
	routerFile := filepath.Join(app, "router.go")
	raw, err := os.ReadFile(routerFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routerFile, append(raw, []byte("\n// my edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = mgo(t, bin, app, "swap", "router", "chi")
	if err == nil {
		t.Fatalf("swap overwrote an edited file:\n%s", out)
	}
	if !strings.Contains(out, "router.go") {
		t.Fatalf("refusal did not name the edited file:\n%s", out)
	}

	// diff sees the same truth the guard enforced.
	out, err = mgo(t, bin, app, "diff")
	if err != nil {
		t.Fatalf("mgo diff: %v\n%s", err, out)
	}
	if !strings.Contains(out, "router.go") || !strings.Contains(out, "modified manually") {
		t.Fatalf("diff did not report the edited file:\n%s", out)
	}
	if !strings.Contains(out, "handlers.go") || !strings.Contains(out, "unchanged") {
		t.Fatalf("diff did not report unchanged files:\n%s", out)
	}

	// info reads the stack from the manifest.
	out, err = mgo(t, bin, app, "info")
	if err != nil {
		t.Fatalf("mgo info: %v\n%s", err, out)
	}
	if !strings.Contains(out, "stdmux") || !strings.Contains(out, "demo") {
		t.Fatalf("info missing stack details:\n%s", out)
	}

	// doctor: the project is healthy, edits and all.
	out, err = mgo(t, bin, app, "doctor")
	if err != nil {
		t.Fatalf("mgo doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "All clear") {
		t.Fatalf("doctor not clear:\n%s", out)
	}
}
