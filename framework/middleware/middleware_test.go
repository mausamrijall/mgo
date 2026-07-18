package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChainOrder(t *testing.T) {
	var trace []string
	tag := func(s string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace = append(trace, s)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace = append(trace, "h")
	}), tag("a"), tag("b"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if got := strings.Join(trace, ","); got != "a,b,h" {
		t.Fatalf("order = %s, want a,b,h", got)
	}
}

func TestRequestIDGeneratesAndPropagates(t *testing.T) {
	var seen string
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = GetRequestID(r.Context())
	}), RequestID())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if seen == "" {
		t.Fatal("no request id in context")
	}
	if rec.Header().Get(RequestIDHeader) != seen {
		t.Fatal("response header does not match context id")
	}
}

func TestRequestIDTrustsInbound(t *testing.T) {
	var seen string
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = GetRequestID(r.Context())
	}), RequestID())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(RequestIDHeader, "abc123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "abc123" {
		t.Fatalf("id = %q, want abc123", seen)
	}
}

func TestRecover(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}), Recover(log))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestLoggerCapturesStatus(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}), Logger(log))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/tea", nil))
	out := buf.String()
	if !strings.Contains(out, "status=418") || !strings.Contains(out, "path=/tea") {
		t.Fatalf("log line missing fields: %s", out)
	}
}

func TestTimeout(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}), Timeout(10*time.Millisecond))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSecureHeaders(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), SecureHeaders())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing frame options header")
	}
}
