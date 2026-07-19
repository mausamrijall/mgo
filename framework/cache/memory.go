// Package cache is MGO's first-party cache glue: an in-memory driver
// (dev, tests, single-node) and the Remember helper with per-key
// singleflight. Redis/memcached and tiered stores are adapters
// implementing the same contracts.
package cache

import (
	"context"
	"sync"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
)

// Memory is a thread-safe in-memory store implementing Store, Locker and
// Counter. Expired entries are dropped lazily on read and swept when the
// map grows.
type Memory struct {
	mu    sync.Mutex
	items map[string]entry
	sweep int
}

type entry struct {
	value []byte
	exp   time.Time // zero = no expiry
	token uint64    // lock fencing token
}

var (
	_ cachec.Store   = (*Memory)(nil)
	_ cachec.Locker  = (*Memory)(nil)
	_ cachec.Counter = (*Memory)(nil)
)

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{items: map[string]entry{}} }

func (m *Memory) live(e entry) bool { return e.exp.IsZero() || time.Now().Before(e.exp) }

func (m *Memory) at(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}

// Get implements cache.Store.
func (m *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok || !m.live(e) {
		delete(m.items, key)
		return nil, false, nil
	}
	return e.value, true, nil
}

// Set implements cache.Store.
func (m *Memory) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = entry{value: value, exp: m.at(ttl)}
	m.maybeSweep()
	return nil
}

// Delete implements cache.Store.
func (m *Memory) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

// maybeSweep drops expired entries every 1024 writes (called locked).
func (m *Memory) maybeSweep() {
	m.sweep++
	if m.sweep < 1024 {
		return
	}
	m.sweep = 0
	for k, e := range m.items {
		if !m.live(e) {
			delete(m.items, k)
		}
	}
}

var lockTokens uint64 // process-unique fencing tokens (guarded by m.mu)

// TryLock implements cache.Locker.
func (m *Memory) TryLock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := "lock:" + key
	if e, ok := m.items[k]; ok && m.live(e) {
		return nil, false, nil
	}
	lockTokens++
	token := lockTokens
	m.items[k] = entry{exp: m.at(ttl), token: token}
	release := func(context.Context) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if e, ok := m.items[k]; ok && e.token == token {
			delete(m.items, k) // only the holder's release clears it
		}
		return nil
	}
	return release, true, nil
}

// Increment implements cache.Counter.
func (m *Memory) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := "ctr:" + key
	e, ok := m.items[k]
	var n int64
	if ok && m.live(e) {
		n = int64(e.value[0])<<56 | int64(e.value[1])<<48 | int64(e.value[2])<<40 | int64(e.value[3])<<32 |
			int64(e.value[4])<<24 | int64(e.value[5])<<16 | int64(e.value[6])<<8 | int64(e.value[7])
	} else {
		e = entry{exp: m.at(ttl)} // window starts on first increment
	}
	n++
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(n >> (56 - 8*i))
	}
	e.value = buf
	m.items[k] = e
	return n, nil
}
