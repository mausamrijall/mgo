package benchmarks

// The glue-overhead certification: MGO's router adapters must stay
// within 5% of the raw library. Runs as a TEST so it gates CI, not just
// a chart. Ratios are noisy on shared machines, so each side gets a
// best-of-N (min) before comparison.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	mgochi "github.com/mgo-framework/mgo/adapters/router-chi"
	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
)

// measure returns the best (lowest) ns/op of rounds runs.
func measure(h http.Handler, rounds int) float64 {
	req := httptest.NewRequest("GET", "/posts/42", nil)
	w := nopWriter{h: make(http.Header)}
	best := 0.0
	for r := 0; r < rounds; r++ {
		res := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				h.ServeHTTP(w, req)
			}
		})
		ns := float64(res.NsPerOp())
		if best == 0 || ns < best {
			best = ns
		}
	}
	return best
}

func handlerFunc(w http.ResponseWriter, req *http.Request) {
	writePost(w, req.PathValue("id"))
}

func TestGlueOverheadCertification(t *testing.T) {
	if testing.Short() {
		t.Skip("certification benchmark in -short mode")
	}

	raw := chi.NewRouter()
	raw.Get("/posts/{id}", handlerFunc)

	wrapped := mgochi.New()
	wrapped.Get("/posts/{id}", handlerFunc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /posts/{id}", handlerFunc)

	adapted := stdmux.New()
	adapted.HandleFunc("GET /posts/{id}", handlerFunc)

	const rounds = 5
	const limit = 1.05 // ≤5% overhead — the certified bound

	cases := []struct {
		name     string
		raw, mgo http.Handler
	}{
		{"chi", raw, wrapped},
		{"stdmux", mux, adapted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawNs := measure(tc.raw, rounds)
			mgoNs := measure(tc.mgo, rounds)
			ratio := mgoNs / rawNs
			t.Logf("%s: raw %.0f ns/op, mgo %.0f ns/op, ratio %.3f (limit %.2f)",
				tc.name, rawNs, mgoNs, ratio, limit)
			if ratio > limit {
				t.Fatalf("glue overhead %.1f%% exceeds the certified 5%% bound", (ratio-1)*100)
			}
		})
	}
}
