// Command greeter is the Phase 11 extraction demo: the same application
// with the Greeter service in-process or behind gRPC, chosen by config:
//
//	MGO_GREETER_TRANSPORT=local go run .   # monolith (default)
//	MGO_GREETER_TRANSPORT=grpc  go run .   # serves gRPC on :9090 AND consumes it
//
// The HTTP surface (GET /greet/{name}) and its integration tests are
// identical in both modes — extraction changed the wiring, not the app.
package main

import (
	"context"
	"log/slog"
	"os"

	mgogrpc "github.com/mgo-framework/mgo/adapters/grpc-server"
	appc "github.com/mgo-framework/mgo/contracts/app"
	greeterv1 "github.com/mgo-framework/mgo/examples/greeter/gen/greeter/v1"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg, err := conf.NewLoader().DotEnv(".env", true).Env("MGO_").Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	transport := cfg.String("greeter.transport", "local")
	var greeter Greeter
	var providers []appc.Provider

	switch transport {
	case "local":
		greeter = localGreeter{}
	case "grpc":
		grpcAddr := cfg.String("grpc.addr", ":9090")
		srv := mgogrpc.New(mgogrpc.Config{Addr: grpcAddr})
		greeterv1.RegisterGreeterServiceServer(srv, grpcService{impl: localGreeter{}})
		providers = append(providers, runnerProvider{srv})

		conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Error("grpc dial", "error", err)
			os.Exit(1)
		}
		greeter = grpcGreeter{client: greeterv1.NewGreeterServiceClient(conn)}
	default:
		slog.Error("unknown transport", "transport", transport)
		os.Exit(1)
	}
	slog.Info("greeter ready", "transport", transport)

	providers = append(providers, httpserver.Provider("http", buildHTTP(greeter)))
	app := mgo.New(mgo.WithConfig(cfg), mgo.WithProviders(providers...))
	if err := app.Run(context.Background()); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

// runnerProvider registers a prebuilt runner (the gRPC server) with the
// app lifecycle.
type runnerProvider struct{ r appc.Runner }

func (p runnerProvider) Register(app appc.App) error { return nil }
func (p runnerProvider) Boot(ctx context.Context, app appc.App) error {
	app.AddRunner(p.r)
	return nil
}
