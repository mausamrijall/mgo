// Package health aggregates contracts/health checkers into liveness and
// readiness endpoints — the standard kubernetes-shaped pair. First-party
// and stdlib-only: every MGO store adapter already satisfies the Checker
// contract structurally, so wiring is Add("db", store) and done.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	healthc "github.com/mgo-framework/mgo/contracts/health"
)

// Registry holds named checkers and renders their combined status.
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]healthc.Checker
	timeout  time.Duration
}

// New builds a registry; perCheckTimeout <= 0 defaults to 2s.
func New(perCheckTimeout time.Duration) *Registry {
	if perCheckTimeout <= 0 {
		perCheckTimeout = 2 * time.Second
	}
	return &Registry{checkers: map[string]healthc.Checker{}, timeout: perCheckTimeout}
}

// Add registers a checker under a stable name (db, cache, broker, ...).
func (r *Registry) Add(name string, c healthc.Checker) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[name] = c
	return r
}

// Result is one checker's outcome.
type Result struct {
	Name    string        `json:"name"`
	Healthy bool          `json:"healthy"`
	Error   string        `json:"error,omitempty"`
	Took    time.Duration `json:"took_ns"`
}

// Check runs every checker in parallel, each bounded by the per-check
// timeout, and returns results sorted by name.
func (r *Registry) Check(ctx context.Context) []Result {
	r.mu.RLock()
	names := make([]string, 0, len(r.checkers))
	for name := range r.checkers {
		names = append(names, name)
	}
	checkers := make([]healthc.Checker, len(names))
	for i, name := range names {
		checkers[i] = r.checkers[name]
	}
	r.mu.RUnlock()

	results := make([]Result, len(names))
	var wg sync.WaitGroup
	for i := range names {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			start := time.Now()
			err := checkers[i].Health(cctx)
			results[i] = Result{Name: names[i], Healthy: err == nil, Took: time.Since(start)}
			if err != nil {
				results[i].Error = err.Error()
			}
		}(i)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

// LiveHandler is the liveness probe: 200 if the process serves requests
// at all. Dependencies deliberately excluded — a dead database must not
// get the pod restarted.
func LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(`{"status":"live"}` + "\n"))
	}
}

// ReadyHandler is the readiness probe: runs every checker; 200 with
// per-check detail when all pass, 503 with the same detail when any
// fails (traffic should be routed away, not the process killed).
func (r *Registry) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		results := r.Check(req.Context())
		ready := true
		for _, res := range results {
			if !res.Healthy {
				ready = false
				break
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"ready": ready, "checks": results})
	}
}
