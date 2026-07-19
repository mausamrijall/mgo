package middleware

import (
	"net"
	"net/http"
	"strconv"
	"time"

	cachec "github.com/mgo-framework/mgo/contracts/cache"
	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// ThrottleConfig tunes Throttle. Zero values get sane defaults.
type ThrottleConfig struct {
	// Limit is the max requests per window per key (default 60).
	Limit int64
	// Window is the fixed counting window (default 1 minute).
	Window time.Duration
	// Key derives the client key; default is the remote IP.
	Key func(r *http.Request) string
}

// Throttle rate-limits requests with fixed windows over any cache
// Counter — framework/cache.Memory for one node, cache-redis to share
// limits across a fleet. Over-limit requests get 429 with Retry-After.
func Throttle(c cachec.Counter, cfg ThrottleConfig) routerc.Middleware {
	if cfg.Limit <= 0 {
		cfg.Limit = 60
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.Key == nil {
		cfg.Key = func(r *http.Request) string {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return r.RemoteAddr
			}
			return host
		}
	}
	retryAfter := strconv.Itoa(int(cfg.Window.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, err := c.Increment(r.Context(), "throttle:"+cfg.Key(r), cfg.Window)
			if err != nil {
				next.ServeHTTP(w, r) // fail open: a broken cache must not take the API down
				return
			}
			if n > cfg.Limit {
				w.Header().Set("Retry-After", retryAfter)
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
