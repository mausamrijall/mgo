// Command routing demonstrates MGO's router glue: the SAME handlers and
// middleware stack served by chi or the stdlib mux, chosen by config:
//
//	MGO_HTTP_ROUTER=chi    go run .   # default
//	MGO_HTTP_ROUTER=stdmux go run .
//
// Handlers read params with r.PathValue (both routers populate it) and
// respond with plain-function helpers — no mgo.Ctx anywhere. Routes are
// registered with each router's NATIVE API; MGO only glues the resulting
// handler into the app lifecycle.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	mgocors "github.com/mgo-framework/mgo/adapters/middleware-cors"
	mgochi "github.com/mgo-framework/mgo/adapters/router-chi"
	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
	"github.com/mgo-framework/mgo/framework/middleware"
	"github.com/mgo-framework/mgo/framework/web"
)

func main() {
	cfg, err := conf.NewLoader().DotEnv(".env", true).Env("MGO_").Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	router, err := buildRouter(cfg.String("http.router", "chi"))
	if err != nil {
		slog.Error("router", "error", err)
		os.Exit(1)
	}

	// Optional capability: list routes if this router can enumerate them.
	if rl, ok := router.(routerc.RouteLister); ok {
		for _, rt := range rl.Routes() {
			slog.Info("route", "method", rt.Method, "pattern", rt.Pattern)
		}
	}

	app := mgo.New(
		mgo.WithConfig(cfg),
		mgo.WithProviders(httpserver.Provider("http", router)),
	)
	if err := app.Run(context.Background()); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

// buildRouter wires the shared middleware and routes onto the configured
// router — each through its own native registration API.
func buildRouter(name string) (routerc.Router, error) {
	mw := []routerc.Middleware{
		middleware.RequestID(),
		middleware.Recover(nil),
		middleware.Logger(nil),
		middleware.SecureHeaders(),
		mgocors.New(mgocors.Config{AllowedOrigins: []string{"*"}}),
	}

	switch name {
	case "chi":
		r := mgochi.New()
		r.Use(mw...)
		r.Get("/hello/{name}", helloHandler) // chi's API
		r.Post("/echo", echoHandler)
		return r, nil
	case "stdmux":
		r := stdmux.New()
		r.Use(mw...)
		r.HandleFunc("GET /hello/{name}", helloHandler) // stdlib's API
		r.HandleFunc("POST /echo", echoHandler)
		return r, nil
	default:
		return nil, fmt.Errorf("unknown router %q (want chi or stdmux)", name)
	}
}

// The handlers are ordinary net/http code, shared verbatim across routers.

func helloHandler(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, http.StatusOK, map[string]string{
		"hello":      r.PathValue("name"),
		"request_id": middleware.GetRequestID(r.Context()),
	})
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := web.Bind(r, &body); err != nil {
		web.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	web.JSON(w, http.StatusOK, body)
}
