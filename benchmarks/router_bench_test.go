// Package benchmarks holds MGO's published performance comparisons and
// the gates that keep them honest. Run:
//
//	go test -bench=. -benchtime=2000000x -count=5 ./...
//
// Methodology: one identical endpoint (GET /posts/{id} returning a small
// JSON object) served by each net/http-compatible framework, invoked
// in-process with a no-op ResponseWriter so the router+handler path
// dominates. Fiber is excluded: it runs on fasthttp, so an in-process
// net/http comparison would measure the adapter shim, not fiber.
package benchmarks

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v4"
	mgochi "github.com/mgo-framework/mgo/adapters/router-chi"
	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	"github.com/mgo-framework/mgo/framework/middleware"
)

type post struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// nopWriter keeps allocation noise out of the measurement.
type nopWriter struct{ h http.Header }

func (w nopWriter) Header() http.Header       { return w.h }
func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriter) WriteHeader(int)             {}

func bench(b *testing.B, h http.Handler) {
	b.Helper()
	req := httptest.NewRequest("GET", "/posts/42", nil)
	w := nopWriter{h: make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, req)
	}
}

func writePost(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post{ID: id, Title: "hello world"})
}

// ---- raw libraries ----

func BenchmarkRawChi(b *testing.B) {
	r := chi.NewRouter()
	r.Get("/posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		writePost(w, chi.URLParam(req, "id"))
	})
	bench(b, r)
}

func BenchmarkRawStdmux(b *testing.B) {
	m := http.NewServeMux()
	m.HandleFunc("GET /posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		writePost(w, req.PathValue("id"))
	})
	bench(b, m)
}

// ---- MGO adapters, bare (certified vs raw) ----

func BenchmarkMGOChi(b *testing.B) {
	r := mgochi.New()
	r.Get("/posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		writePost(w, req.PathValue("id"))
	})
	bench(b, r)
}

func BenchmarkMGOStdmux(b *testing.B) {
	r := stdmux.New()
	r.HandleFunc("GET /posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		writePost(w, req.PathValue("id"))
	})
	bench(b, r)
}

// ---- MGO with the full production middleware stack ----

func BenchmarkMGOChiFullStack(b *testing.B) {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := mgochi.New()
	r.Use(
		middleware.RequestID(),
		middleware.Recover(discard),
		middleware.Logger(discard),
		middleware.SecureHeaders(),
	)
	r.Get("/posts/{id}", func(w http.ResponseWriter, req *http.Request) {
		writePost(w, req.PathValue("id"))
	})
	bench(b, r)
}

// ---- other frameworks (their own idioms, same work) ----

func BenchmarkGin(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/posts/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, post{ID: c.Param("id"), Title: "hello world"})
	})
	bench(b, r)
}

func BenchmarkEcho(b *testing.B) {
	e := echo.New()
	e.GET("/posts/:id", func(c echo.Context) error {
		return c.JSON(http.StatusOK, post{ID: c.Param("id"), Title: "hello world"})
	})
	bench(b, e)
}
