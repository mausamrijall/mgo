// Command authdemo is the Phase 6 exit demo: session login + JWT API +
// ability-gated admin route, on chi or stdmux via config:
//
//	MGO_HTTP_ROUTER=chi    go run .   # default
//	MGO_HTTP_ROUTER=stdmux go run .
//
// Accounts: admin@example.com/admin123, user@example.com/user123.
//
//	POST /login    {email,password}  session login (CSRF-protected)
//	GET  /me                         who am I (session)
//	POST /logout                     end session
//	POST /token    {email,password}  mint a JWT
//	GET  /api/data                   bearer-only API
//	GET  /admin                      session OR bearer, "admin" ability
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
)

func main() {
	cfg, err := conf.NewLoader().DotEnv(".env", true).Env("MGO_").Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	secret := []byte(cfg.String("auth.secret", "dev-secret-change-me-32-bytes-min!!"))
	a, err := newApp(secret)
	if err != nil {
		slog.Error("app", "error", err)
		os.Exit(1)
	}

	router := cfg.String("http.router", "chi")
	slog.Info("authdemo ready", "router", router)

	appKernel := mgo.New(
		mgo.WithConfig(cfg),
		mgo.WithProviders(httpserver.Provider("http", a.handler(router))),
	)
	if err := appKernel.Run(context.Background()); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}
