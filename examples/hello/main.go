// Command hello is the minimal MGO application: the kernel serving a
// stdlib http.ServeMux through the app lifecycle. This is what a
// `mgo new hello --preset=minimal` project reduces to — and what remains
// if you delete every adapter: plain Go.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	appc "github.com/mgo-framework/mgo/contracts/app"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
)

func main() {
	app := mgo.New(mgo.WithProviders(webProvider{}))
	if err := app.Run(context.Background()); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

// webProvider mounts routes and registers the HTTP runner.
type webProvider struct{}

func (webProvider) Register(app appc.App) error { return nil }

func (webProvider) Boot(ctx context.Context, app appc.App) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello from MGO — the Go Application Platform")
	})

	var cfg httpserver.Config
	if err := app.Config().Bind("http", &cfg); err != nil {
		return err
	}
	app.AddRunner(httpserver.New("http", mux, cfg))
	return nil
}
