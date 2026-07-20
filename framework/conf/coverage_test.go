package conf_test

// Coverage top-ups for accessor fallbacks, coercions, Value, and Bind
// error paths.

import (
	"testing"

	"github.com/mgo-framework/mgo/framework/conf"
)

func load(t *testing.T, tree map[string]any) interface {
	Value(string) any
	String(string, ...string) string
	Int(string, ...int) int
	Bool(string, ...bool) bool
	Float(string, ...float64) float64
	Has(string) bool
	Bind(string, any) error
} {
	t.Helper()
	cfg, err := conf.NewLoader().Overrides(tree).Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestValueAndHas(t *testing.T) {
	cfg := load(t, map[string]any{"a": map[string]any{"b": 7}})
	if v := cfg.Value("a.b"); v != 7 {
		t.Fatalf("Value = %v", v)
	}
	if cfg.Value("missing") != nil {
		t.Fatal("missing Value should be nil")
	}
	if cfg.Has("missing") || !cfg.Has("a.b") {
		t.Fatal("Has wrong")
	}
}

func TestAccessorFallbacksAndCoercion(t *testing.T) {
	cfg := load(t, map[string]any{
		"s": 42, "i": "17", "b": "true", "f": "2.5",
		"badint": "zzz",
	})
	// Coercions across types.
	if cfg.String("s") != "42" {
		t.Fatalf("String coercion = %q", cfg.String("s"))
	}
	if cfg.Int("i") != 17 {
		t.Fatalf("Int coercion = %d", cfg.Int("i"))
	}
	if cfg.Bool("b") != true {
		t.Fatal("Bool coercion failed")
	}
	if cfg.Float("f") != 2.5 {
		t.Fatalf("Float coercion = %v", cfg.Float("f"))
	}
	// Fallbacks on absent keys.
	if cfg.String("nope", "dflt") != "dflt" || cfg.Int("nope", 9) != 9 ||
		cfg.Bool("nope", true) != true || cfg.Float("nope", 1.5) != 1.5 {
		t.Fatal("fallbacks not applied")
	}
	// Unparseable values fall back too.
	if cfg.Int("badint", 3) != 3 {
		t.Fatal("bad int did not fall back")
	}
	// No fallback + missing = zero values.
	if cfg.String("nope") != "" || cfg.Int("nope") != 0 || cfg.Bool("nope") || cfg.Float("nope") != 0 {
		t.Fatal("zero-value defaults wrong")
	}
}

func TestBindErrors(t *testing.T) {
	cfg := load(t, map[string]any{"http": map[string]any{"addr": ":1", "port": "notanint"}})

	// Non-pointer target.
	var notPtr struct{}
	if err := cfg.Bind("http", notPtr); err == nil {
		t.Fatal("bind to non-pointer must error")
	}
	// Type mismatch inside the tree.
	var typed struct {
		Port int `conf:"port"`
	}
	if err := cfg.Bind("http", &typed); err == nil {
		t.Fatal("bind of non-numeric string into int must error")
	}
	// Binding an absent subtree is not an error — zero struct.
	var empty struct {
		Addr string `conf:"addr"`
	}
	if err := cfg.Bind("absent", &empty); err != nil {
		t.Fatalf("absent subtree bind errored: %v", err)
	}
}
