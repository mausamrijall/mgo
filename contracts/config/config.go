// Package config defines the MGO configuration contracts.
//
// The kernel owns configuration *policy* — layered precedence, dot-key
// access, typed reads, struct binding — while *parsing* of formats beyond
// JSON/env is delegated to adapters (koanf, viper, ...) per the glue
// philosophy. Precedence is a kernel invariant (doc 06 §8):
//
//	defaults < files < .env < environment < explicit overrides
package config

// Config is a read view over merged configuration layers.
type Config interface {
	// Has reports whether a value exists at the dot-separated path.
	Has(path string) bool

	// Value returns the raw value at path, or nil when absent.
	Value(path string) any

	// String, Int, Bool, Float return coerced values with an optional
	// fallback used when the path is absent or not coercible.
	String(path string, fallback ...string) string
	Int(path string, fallback ...int) int
	Bool(path string, fallback ...bool) bool
	Float(path string, fallback ...float64) float64

	// Bind decodes the subtree at path into target (a struct pointer).
	// Providers use this at register time so misconfiguration fails at
	// boot, not at first request.
	Bind(path string, target any) error

	// Sub returns a view rooted at path (empty view when absent).
	Sub(path string) Config
}

// Source supplies one layer of configuration. Adapters implement this to
// contribute parsed file formats or remote stores. Layers are merged in
// the order they are added to the loader; later layers win.
type Source interface {
	// Load returns a nested map[string]any tree of configuration values.
	Load() (map[string]any, error)
}
