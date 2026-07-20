package main

// The extraction pattern. The application depends on ONE Go interface
// (Greeter). "Monolith" wires the local implementation; "microservice"
// wires a gRPC client implementing the same interface. The HTTP surface
// and its tests never change — that is the Phase 11 exit.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	greeterv1 "github.com/mgo-framework/mgo/examples/greeter/gen/greeter/v1"
	"github.com/mgo-framework/mgo/framework/middleware"
	"github.com/mgo-framework/mgo/framework/web"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNameRequired is the domain error, identical across transports.
var ErrNameRequired = errors.New("name required")

// Greeter is the service boundary — plain Go, no transport in sight.
type Greeter interface {
	Greet(ctx context.Context, name string) (string, error)
}

// ---- local implementation (the "monolith" wiring) ----

type localGreeter struct{}

func (localGreeter) Greet(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", ErrNameRequired
	}
	return fmt.Sprintf("Hello, %s!", name), nil
}

// ---- gRPC server adapter: serves any Greeter over the wire ----

type grpcService struct {
	greeterv1.UnimplementedGreeterServiceServer
	impl Greeter
}

func (s grpcService) Greet(ctx context.Context, req *greeterv1.GreetRequest) (*greeterv1.GreetResponse, error) {
	greeting, err := s.impl.Greet(ctx, req.GetName())
	if err != nil {
		if errors.Is(err, ErrNameRequired) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &greeterv1.GreetResponse{Greeting: greeting}, nil
}

// ---- gRPC client adapter: a Greeter backed by the wire ----

type grpcGreeter struct {
	client greeterv1.GreeterServiceClient
}

func (g grpcGreeter) Greet(ctx context.Context, name string) (string, error) {
	resp, err := g.client.Greet(ctx, &greeterv1.GreetRequest{Name: name})
	if err != nil {
		if status.Code(err) == codes.InvalidArgument {
			return "", ErrNameRequired // domain error restored across the wire
		}
		return "", err
	}
	return resp.GetGreeting(), nil
}

// ---- the HTTP surface: written once, transport-blind ----

func buildHTTP(g Greeter) http.Handler {
	r := stdmux.New()
	r.Use(middleware.RequestID(), middleware.Recover(nil))
	r.HandleFunc("GET /greet/{name}", func(w http.ResponseWriter, req *http.Request) {
		greeting, err := g.Greet(req.Context(), req.PathValue("name"))
		if err != nil {
			if errors.Is(err, ErrNameRequired) {
				web.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			web.Error(w, http.StatusBadGateway, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, map[string]string{"greeting": greeting})
	})
	r.HandleFunc("GET /greet/", func(w http.ResponseWriter, req *http.Request) {
		web.Error(w, http.StatusBadRequest, ErrNameRequired.Error())
	})
	return r
}
