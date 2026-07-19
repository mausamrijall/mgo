package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/cache"
	"github.com/mgo-framework/mgo/framework/middleware"
)

func TestThrottleLimitsPerKeyWindow(t *testing.T) {
	h := middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
		middleware.Throttle(cache.NewMemory(), middleware.ThrottleConfig{Limit: 3, Window: 80 * time.Millisecond}),
	)
	hit := func(addr string) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := range 3 {
		if code := hit("1.2.3.4:1000"); code != 200 {
			t.Fatalf("request %d = %d, want 200", i+1, code)
		}
	}
	if code := hit("1.2.3.4:1000"); code != http.StatusTooManyRequests {
		t.Fatalf("over-limit = %d, want 429", code)
	}
	// A different client is unaffected.
	if code := hit("5.6.7.8:1000"); code != 200 {
		t.Fatalf("other client = %d, want 200", code)
	}
	// Window resets.
	time.Sleep(100 * time.Millisecond)
	if code := hit("1.2.3.4:1000"); code != 200 {
		t.Fatalf("after window = %d, want 200", code)
	}
}
