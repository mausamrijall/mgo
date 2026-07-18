// Package stdmux adapts the stdlib http.ServeMux to MGO's router contract.
// The ServeMux is embedded, so its native API is the API: register routes
// with Handle/HandleFunc and Go 1.22 "METHOD /path/{param}" patterns; read
// params with r.PathValue. The adapter adds only what the contract needs —
// Use, Mount, and Routes metadata — on top.
package stdmux

import (
	"net/http"
	"strings"
	"sync"

	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Router is an http.ServeMux with middleware, mounting, and route
// metadata. The zero value is not usable; call New.
type Router struct {
	*http.ServeMux

	mu     sync.Mutex
	mw     []routerc.Middleware
	routes []routerc.Route

	buildOnce sync.Once
	handler   http.Handler
}

var (
	_ routerc.Router      = (*Router)(nil)
	_ routerc.RouteLister = (*Router)(nil)
)

// New returns an empty Router.
func New() *Router {
	return &Router{ServeMux: http.NewServeMux()}
}

// Handle registers a handler with ServeMux semantics and records route
// metadata. Same signature and behavior as http.ServeMux.Handle.
func (r *Router) Handle(pattern string, h http.Handler) {
	r.record(pattern)
	r.ServeMux.Handle(pattern, h)
}

// HandleFunc registers a handler function with ServeMux semantics and
// records route metadata.
func (r *Router) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.record(pattern)
	r.ServeMux.HandleFunc(pattern, h)
}

// Use implements contracts/router.Router. It panics if called after the
// router has started serving — middleware must be set up front, matching
// chi's behavior so apps stay portable across adapters.
func (r *Router) Use(mw ...routerc.Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handler != nil {
		panic("stdmux: Use called after the router started serving")
	}
	r.mw = append(r.mw, mw...)
}

// Mount implements contracts/router.Router: h is reachable at pattern and
// all subpaths, with r.URL.Path passed through unmodified.
func (r *Router) Mount(pattern string, h http.Handler) {
	p := strings.TrimSuffix(pattern, "/")
	if p == "" {
		r.record("/")
		r.ServeMux.Handle("/", h)
		return
	}
	r.record(p + "/")
	r.ServeMux.Handle(p, h)
	// The trailing-slash pattern matches the whole subtree. Registering
	// the exact pattern too avoids ServeMux's 301 redirect on /p.
	r.ServeMux.Handle(p+"/", h)
}

// Routes implements the contracts/router.RouteLister capability.
func (r *Router) Routes() []routerc.Route {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]routerc.Route, len(r.routes))
	copy(out, r.routes)
	return out
}

// ServeHTTP applies the middleware chain (built once, first middleware
// outermost) and delegates to the ServeMux.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.buildOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		h := http.Handler(r.ServeMux)
		for i := len(r.mw) - 1; i >= 0; i-- {
			h = r.mw[i](h)
		}
		r.handler = h
	})
	r.handler.ServeHTTP(w, req)
}

// record parses a ServeMux pattern "[METHOD ][HOST]/PATH" into metadata.
func (r *Router) record(pattern string) {
	method, path := "", pattern
	if before, after, ok := strings.Cut(pattern, " "); ok && !strings.Contains(before, "/") {
		method, path = before, strings.TrimSpace(after)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, routerc.Route{Method: method, Pattern: path})
}
