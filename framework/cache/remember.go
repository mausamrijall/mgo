package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
)

// group is a minimal in-process singleflight: concurrent callers of the
// same key share one execution. ~30 lines beats a dependency.
type group struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	done chan struct{}
	val  []byte
	err  error
}

var flight = group{calls: map[string]*call{}}

func (g *group) do(key string, fn func() ([]byte, error)) ([]byte, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &call{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(c.done)
	return c.val, c.err
}

// Remember returns the cached T under key, or computes it with fn, caches
// it for ttl, and returns it. Concurrent misses for the same key collapse
// into ONE fn call per process (the herd guarantee); values are JSON on
// the wire so any Store backend works.
func Remember[T any](ctx context.Context, s cachec.Store, key string, ttl time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	if raw, ok, err := s.Get(ctx, key); err != nil {
		return zero, err
	} else if ok {
		var v T
		if err := json.Unmarshal(raw, &v); err == nil {
			return v, nil
		}
		// Corrupt entry: fall through and recompute.
	}

	raw, err := flight.do(key, func() ([]byte, error) {
		// Re-check inside the flight: another caller may have filled it.
		if raw, ok, err := s.Get(ctx, key); err == nil && ok {
			return raw, nil
		}
		v, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if err := s.Set(ctx, key, raw, ttl); err != nil {
			return nil, err
		}
		return raw, nil
	})
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, err
	}
	return v, nil
}

// Forget removes a remembered key.
func Forget(ctx context.Context, s cachec.Store, key string) error {
	return s.Delete(ctx, key)
}
