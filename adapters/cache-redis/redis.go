// Package mgoredis adapts go-redis to MGO's cache contract: Store, plus
// the Locker (SET NX + compare-and-delete) and Counter (atomic
// INCR+PEXPIRE) capabilities. The *redis.Client is embedded — go-redis's
// native API stays fully available for everything beyond the contract.
package mgoredis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
	"github.com/redis/go-redis/v9"
)

// Store wraps a go-redis client with the cache contract.
type Store struct {
	*redis.Client
}

var (
	_ cachec.Store   = (*Store)(nil)
	_ cachec.Locker  = (*Store)(nil)
	_ cachec.Counter = (*Store)(nil)
)

// New wraps an existing client (you configure pooling, TLS, cluster —
// go-redis's business, not MGO's).
func New(client *redis.Client) *Store { return &Store{Client: client} }

// Get implements cache.Store.
func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	raw, err := s.Client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// Set implements cache.Store. ttl <= 0 stores without expiry.
func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 0
	}
	return s.Client.Set(ctx, key, value, ttl).Err()
}

// Delete implements cache.Store.
func (s *Store) Delete(ctx context.Context, key string) error {
	return s.Client.Del(ctx, key).Err()
}

// releaseScript deletes the lock only if the caller still holds it.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`)

// TryLock implements cache.Locker via SET NX PX with a random holder
// token; release is compare-and-delete so an expired-and-reacquired lock
// is never deleted by the old holder.
func (s *Store) TryLock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, bool, error) {
	var tok [16]byte
	rand.Read(tok[:])
	token := hex.EncodeToString(tok[:])

	ok, err := s.Client.SetNX(ctx, "lock:"+key, token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	release := func(ctx context.Context) error {
		return releaseScript.Run(ctx, s.Client, []string{"lock:" + key}, token).Err()
	}
	return release, true, nil
}

// incrScript increments and starts the ttl window on first increment.
var incrScript = redis.NewScript(`
local v = redis.call("INCR", KEYS[1])
if v == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return v`)

// Increment implements cache.Counter atomically.
func (s *Store) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return incrScript.Run(ctx, s.Client, []string{"ctr:" + key}, ttl.Milliseconds()).Int64()
}
