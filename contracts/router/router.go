// Package router defines MGO's routing integration points — and nothing
// more. There are deliberately no Get/Post/Group verb methods here: users
// build routes with the chosen router's native API (chi.NewRouter(),
// http.NewServeMux(), ...) and MGO only needs the three capabilities below
// to glue that router into the application lifecycle.
//
// The contract is shaped after stdlib idiom so that good routers satisfy
// it natively: *chi.Mux implements Router with zero adapter code.
package router

import "net/http"

// Middleware is the stdlib-shaped middleware function that the entire Go
// ecosystem already understands. MGO defines no other middleware type.
type Middleware = func(http.Handler) http.Handler

// Router is everything MGO's glue needs from a router:
//
//   - serve requests (http.Handler — the kernel's httpserver runner takes it),
//   - accept cross-cutting middleware (Use),
//   - compose sub-handlers under a path prefix (Mount — modules use this).
//
// Semantics required of implementations:
//
//   - Use appends middleware; the first middleware registered is the
//     outermost (runs first). Use must be called before routes are served.
//   - Mount makes h reachable at pattern and all subpaths. r.URL.Path is
//     passed through unmodified (no prefix stripping at the contract level).
type Router interface {
	http.Handler
	Use(mw ...Middleware)
	Mount(pattern string, h http.Handler)
}

// Route is the metadata record for one registered route. It feeds
// diagnostics (route:list) and, later, OpenAPI generation.
type Route struct {
	// Method is the HTTP method, or "" when the route matches any method
	// (e.g. a mounted sub-handler).
	Method string
	// Pattern is the route pattern in the underlying router's own syntax
	// (chi: /users/{id}, stdmux: GET /users/{id} normalized to path part).
	Pattern string
}

// RouteLister is an optional capability: adapters that can enumerate their
// routes implement it, and MGO tooling discovers it by type assertion.
// A router without it still works everywhere — you only lose route:list.
type RouteLister interface {
	Routes() []Route
}
