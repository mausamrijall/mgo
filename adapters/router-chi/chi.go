// Package mgochi adapts go-chi/chi to MGO's router contract — barely.
// *chi.Mux natively satisfies contracts/router.Router (its ServeHTTP, Use,
// and Mount already have the contract signatures), so this package only
// adds the Routes metadata capability. Use chi's own API for everything:
//
//	r := mgochi.New()
//	r.Get("/users/{id}", handler)   // chi's method, not MGO's
//	r.Route("/admin", func(r chi.Router) { ... })
//
// Params read via chi.URLParam or, on Go 1.22+, r.PathValue — chi
// populates both.
package mgochi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Router is a *chi.Mux (embedded — the full chi API is the API) plus the
// contracts/router.RouteLister capability.
type Router struct {
	*chi.Mux
}

var (
	_ routerc.Router      = (*Router)(nil)
	_ routerc.RouteLister = (*Router)(nil)
)

// New returns a Router wrapping a fresh chi mux.
func New() *Router { return &Router{chi.NewRouter()} }

// Wrap adapts an existing chi mux you built elsewhere.
func Wrap(m *chi.Mux) *Router { return &Router{m} }

// Routes implements the contracts/router.RouteLister capability by
// walking chi's route tree. (This shadows chi's own Routes method, which
// returns chi types; reach it via the embedded Mux if needed.)
func (r *Router) Routes() []routerc.Route {
	var out []routerc.Route
	_ = chi.Walk(r.Mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// chi.Walk renders mounted subtrees as /prefix/*; normalize the
		// double-slash artifacts it can produce.
		out = append(out, routerc.Route{Method: method, Pattern: strings.ReplaceAll(route, "//", "/")})
		return nil
	})
	return out
}
