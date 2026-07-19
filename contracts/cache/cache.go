// Package cache defines MGO's caching integration points. Values are raw
// bytes — encoding is the caller's business (framework/cache.Remember
// uses JSON). Tags, tiering, and stampede layers come later; the base
// contract stays this small so every backend can implement it exactly.
package cache

import (
	"context"
	"time"
)

// Store is the base key-value contract every cache driver implements.
type Store interface {
	// Get returns the value and whether the key exists (and is fresh).
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Set stores value for ttl; ttl <= 0 means no expiry.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes the key (no error when absent).
	Delete(ctx context.Context, key string) error
}

// Locker is an optional capability: distributed try-locks for
// OnOneServer scheduling, cross-process singleflight, and the like.
// Discovered by type assertion, like every MGO capability.
type Locker interface {
	// TryLock acquires key for ttl. ok=false means someone else holds it.
	// release is only non-nil when ok; it is safe to call once.
	TryLock(ctx context.Context, key string, ttl time.Duration) (release func(context.Context) error, ok bool, err error)
}

// Counter is an optional capability: atomic fixed-window counters, the
// primitive behind rate limiting.
type Counter interface {
	// Increment adds 1 to key and returns the new value. The ttl window
	// starts when the key is first created.
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
}
