// Package health defines MGO's health-check integration point. It is one
// method on purpose: contracts/orm.HealthChecker and every store/broker
// adapter already satisfy it structurally — no imports, no adaptation.
package health

import "context"

// Checker reports whether a dependency is usable. A nil error means
// healthy. Implementations must respect ctx deadlines: the aggregator
// runs checks with a timeout.
type Checker interface {
	Health(ctx context.Context) error
}

// CheckerFunc adapts a function to Checker.
type CheckerFunc func(ctx context.Context) error

// Health implements Checker.
func (f CheckerFunc) Health(ctx context.Context) error { return f(ctx) }
