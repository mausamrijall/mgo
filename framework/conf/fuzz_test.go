package conf_test

import (
	"testing"

	"github.com/mgo-framework/mgo/framework/conf"
)

// FuzzAccessorPaths: arbitrary dot-paths against a fixed tree must never
// panic, whatever the accessor.
func FuzzAccessorPaths(f *testing.F) {
	cfg, err := conf.NewLoader().Overrides(map[string]any{
		"a": map[string]any{"b": map[string]any{"c": 1}},
		"s": "str", "n": 3.5, "t": true,
		"list": []any{1, "two", map[string]any{"x": 1}},
	}).Load()
	if err != nil {
		f.Fatal(err)
	}

	f.Add("a.b.c")
	f.Add("")
	f.Add("...")
	f.Add("a..b")
	f.Add("list.0.x")
	f.Add("s.deeper.than.a.string")
	f.Add(".leading")
	f.Add("trailing.")

	f.Fuzz(func(t *testing.T, path string) {
		_ = cfg.Value(path)
		_ = cfg.Has(path)
		_ = cfg.String(path, "d")
		_ = cfg.Int(path, 1)
		_ = cfg.Bool(path, false)
		_ = cfg.Float(path, 1.0)
		_ = cfg.Sub(path)
		var target struct{ C int }
		_ = cfg.Bind(path, &target)
	})
}
