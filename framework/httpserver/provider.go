// Provider glue: one line to put any http.Handler — a chi mux, a stdlib
// mux, anything — into the app lifecycle. This is MGO's `UseRouter`:
//
//	router := chi.NewRouter()          // the router's native API
//	router.Get("/", home)
//	app := mgo.New(mgo.WithProviders(httpserver.Provider("http", router)))
package httpserver

import (
	"context"
	"net/http"

	appc "github.com/mgo-framework/mgo/contracts/app"
)

// Provider returns a provider that, at boot, binds the config section
// `name` into Config and registers the handler as an HTTP runner named
// `name`. Multiple Providers with distinct names run multiple listeners.
func Provider(name string, h http.Handler) appc.Provider {
	return &provider{name: name, handler: h}
}

type provider struct {
	name    string
	handler http.Handler
}

func (p *provider) Register(app appc.App) error { return nil }

func (p *provider) Boot(ctx context.Context, app appc.App) error {
	var cfg Config
	if err := app.Config().Bind(p.name, &cfg); err != nil {
		return err
	}
	app.AddRunner(New(p.name, p.handler, cfg))
	return nil
}
