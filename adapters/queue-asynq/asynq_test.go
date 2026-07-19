package mgoasynq_test

// Conformance against miniredis when possible, or a real Redis via
// REDIS_ADDR. asynq leans on Lua and blocking ops that miniredis mostly
// supports; if the environment can't run it, the suite skips loudly
// rather than passing silently.

import (
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	mgoasynq "github.com/mgo-framework/mgo/adapters/queue-asynq"
	"github.com/mgo-framework/mgo/contracts/queuetest"
)

func redisOpt(t *testing.T) asynq.RedisConnOpt {
	t.Helper()
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return asynq.RedisClientOpt{Addr: addr}
	}
	mr := miniredis.RunT(t)
	// asynq schedules with real timestamps; advance miniredis's clock.
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				mr.FastForward(20 * time.Millisecond)
			}
		}
	}()
	return asynq.RedisClientOpt{Addr: mr.Addr()}
}

func TestConformance(t *testing.T) {
	queuetest.Run(t, func(t *testing.T) queuetest.Pair {
		opt := redisOpt(t)
		client := mgoasynq.NewClient(opt)
		t.Cleanup(func() { client.Close() })
		worker := mgoasynq.NewWorker(opt, mgoasynq.Config{
			Concurrency:              2,
			RetryDelay:               20 * time.Millisecond,
			DelayedTaskCheckInterval: 50 * time.Millisecond,
			ShutdownTimeout:          2 * time.Second,
		})
		return queuetest.Pair{Enqueuer: client, Worker: worker}
	})
}
