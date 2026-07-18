// Package middleware is MGO's stdlib-only middleware set. Every function
// here returns a plain func(http.Handler) http.Handler, so each one works
// with chi, gin (via adapters), stdlib mux, or no router at all — and
// deleting MGO leaves them usable as ordinary Go middleware.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Chain wraps h with mw so that the first middleware listed is the
// outermost (runs first) — the same ordering routers give Use.
func Chain(h http.Handler, mw ...routerc.Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

type ctxKey int

const requestIDKey ctxKey = iota

// RequestIDHeader is the header read and written by RequestID.
const RequestIDHeader = "X-Request-Id"

// RequestID trusts an inbound X-Request-Id or generates a 16-hex-char id,
// stores it in the request context and echoes it on the response.
func RequestID() routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if id == "" {
				var b [8]byte
				rand.Read(b[:]) // crypto/rand never fails post-Go 1.24
				id = hex.EncodeToString(b[:])
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
		})
	}
}

// GetRequestID returns the request id stored by RequestID, or "".
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Recover converts panics into 500 responses and logs the stack. A nil
// logger means slog.Default().
func Recover(log *slog.Logger) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { // preserve stdlib abort semantics
						panic(v)
					}
					l := log
					if l == nil {
						l = slog.Default()
					}
					l.ErrorContext(r.Context(), "panic recovered",
						"error", v,
						"method", r.Method,
						"path", r.URL.Path,
						"request_id", GetRequestID(r.Context()),
						"stack", string(debug.Stack()),
					)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter records the status code and bytes written for Logger.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer
// (flush, hijack, deadlines keep working through the wrapper).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Logger emits one structured line per request: method, path, status,
// bytes, duration and request_id. A nil logger means slog.Default().
func Logger(log *slog.Logger) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			if sw.status == 0 {
				sw.status = http.StatusOK
			}
			l := log
			if l == nil {
				l = slog.Default()
			}
			l.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration", time.Since(start),
				"request_id", GetRequestID(r.Context()),
			)
		})
	}
}

// Timeout aborts handlers that exceed d with a 503, via the stdlib
// http.TimeoutHandler (safe concurrent write semantics included).
func Timeout(d time.Duration) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, http.StatusText(http.StatusServiceUnavailable))
	}
}

// SecureHeaders sets conservative browser-hardening headers. Override any
// of them downstream by setting the header again in a later middleware or
// handler before the first write.
func SecureHeaders() routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	}
}
