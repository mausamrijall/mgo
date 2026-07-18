package conf_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/conf"
)

// TestPrecedenceOrder verifies the kernel invariant:
// defaults < files < .env < environment < explicit overrides.
func TestPrecedenceOrder(t *testing.T) {
	dir := t.TempDir()

	jsonPath := filepath.Join(dir, "app.json")
	os.WriteFile(jsonPath, []byte(`{"app":{"name":"from-file","port":8080,"debug":true}}`), 0o644)

	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("APP_NAME=from-dotenv\n# comment\nAPP_EXTRA=dot\n"), 0o644)

	t.Setenv("MGO_APP_NAME", "from-env")

	cfg, err := conf.NewLoader().
		Defaults(map[string]any{"app.name": "from-defaults", "app.tz": "UTC"}).
		JSONFile(jsonPath, false).
		DotEnv(envPath, false).
		Env("MGO_").
		Overrides(map[string]any{"app.name": "from-override"}).
		Load()
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.String("app.name"); got != "from-override" {
		t.Fatalf("override should win, got %q", got)
	}
	if got := cfg.String("app.tz"); got != "UTC" {
		t.Fatalf("default should survive, got %q", got)
	}
	if got := cfg.Int("app.port"); got != 8080 {
		t.Fatalf("file value lost, got %d", got)
	}
	if got := cfg.String("app.extra"); got != "dot" {
		t.Fatalf("dotenv value lost, got %q", got)
	}
	if !cfg.Bool("app.debug") {
		t.Fatal("file bool lost")
	}
}

func TestEnvBeatsDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("APP_NAME=dotenv\n"), 0o644)
	t.Setenv("MGO_APP_NAME", "process-env")

	cfg, err := conf.NewLoader().DotEnv(envPath, false).Env("MGO_").Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.String("app.name"); got != "process-env" {
		t.Fatalf("process env should beat .env, got %q", got)
	}
}

func TestMissingOptionalFilesSkipped(t *testing.T) {
	cfg, err := conf.NewLoader().
		JSONFile("/nonexistent/app.json", true).
		DotEnv("/nonexistent/.env", true).
		Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Has("anything") {
		t.Fatal("empty config should have nothing")
	}
}

func TestMissingRequiredFileErrors(t *testing.T) {
	_, err := conf.NewLoader().JSONFile("/nonexistent/app.json", false).Load()
	if err == nil {
		t.Fatal("required missing file must error")
	}
}

func TestTypedAccessAndFallbacks(t *testing.T) {
	cfg, err := conf.NewLoader().Defaults(map[string]any{
		"n": "42", "f": "2.5", "b": "true", "s": 7,
	}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Int("n") != 42 || cfg.Float("f") != 2.5 || !cfg.Bool("b") || cfg.String("s") != "7" {
		t.Fatal("string coercions failed")
	}
	if cfg.Int("missing", 9) != 9 || cfg.String("missing", "x") != "x" || !cfg.Bool("missing", true) {
		t.Fatal("fallbacks failed")
	}
}

type httpCfg struct {
	Host    string
	Port    int
	Debug   bool
	Timeout time.Duration `conf:"timeout"`
	Tags    []string
	Limits  map[string]int
	TLS     struct {
		Enabled bool
	} `conf:"tls"`
}

func TestStructBinding(t *testing.T) {
	cfg, err := conf.NewLoader().Defaults(map[string]any{
		"http": map[string]any{
			"host": "0.0.0.0", "port": "9090", "debug": "true",
			"timeout": "30s",
			"tags":    []any{"a", "b"},
			"limits":  map[string]any{"burst": 10},
			"tls":     map[string]any{"enabled": true},
		},
	}).Load()
	if err != nil {
		t.Fatal(err)
	}
	var h httpCfg
	if err := cfg.Bind("http", &h); err != nil {
		t.Fatal(err)
	}
	if h.Host != "0.0.0.0" || h.Port != 9090 || !h.Debug || h.Timeout != 30*time.Second {
		t.Fatalf("bind mismatch: %+v", h)
	}
	if len(h.Tags) != 2 || h.Limits["burst"] != 10 || !h.TLS.Enabled {
		t.Fatalf("bind nested mismatch: %+v", h)
	}
}

func TestBindCoercionFailureErrors(t *testing.T) {
	cfg, _ := conf.NewLoader().Defaults(map[string]any{
		"http": map[string]any{"port": "not-a-number"},
	}).Load()
	var h httpCfg
	if err := cfg.Bind("http", &h); err == nil {
		t.Fatal("bad coercion must error at boot")
	}
}

func TestSubView(t *testing.T) {
	cfg, _ := conf.NewLoader().Defaults(map[string]any{"a.b.c": 1}).Load()
	if got := cfg.Sub("a").Sub("b").Int("c"); got != 1 {
		t.Fatalf("sub view failed, got %d", got)
	}
	if cfg.Sub("missing").Has("x") {
		t.Fatal("missing sub must be empty view")
	}
}
