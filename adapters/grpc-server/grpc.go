// Package mgogrpc adapts google.golang.org/grpc to the MGO app
// lifecycle. The *grpc.Server is embedded — register services with the
// generated RegisterXxxServer functions as always; MGO adds only
// lifecycle (graceful stop inside app shutdown), the standard gRPC
// health service, and panic-safe default interceptors.
package mgogrpc

import (
	"context"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Config for the runner; bound from the "grpc" config section.
type Config struct {
	Addr          string        `conf:"addr" json:"addr"`
	ShutdownGrace time.Duration `conf:"shutdown_grace" json:"shutdown_grace"`
}

// Server wraps *grpc.Server as a contracts/app.Runner.
type Server struct {
	*grpc.Server
	cfg    Config
	health *health.Server

	ready chan struct{}
	addr  string
}

var _ appc.Runner = (*Server)(nil)

// New builds a server with Recover and Log interceptors installed ahead
// of any the caller adds, and the grpc.health.v1 service registered
// (kubelet grpc probes and grpc-health-probe work out of the box).
// Defaults: addr ":9090", shutdown grace 10s.
func New(cfg Config, opts ...grpc.ServerOption) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":9090"
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 10 * time.Second
	}
	opts = append([]grpc.ServerOption{
		grpc.ChainUnaryInterceptor(RecoverUnary(nil), LogUnary(nil)),
	}, opts...)

	s := &Server{Server: grpc.NewServer(opts...), cfg: cfg, health: health.NewServer(), ready: make(chan struct{})}
	healthpb.RegisterHealthServer(s.Server, s.health)
	return s
}

// Name implements contracts/app.Runner.
func (s *Server) Name() string { return "grpc" }

// Addr returns the bound address once listening (":0"-friendly tests).
func (s *Server) Addr(ctx context.Context) (string, error) {
	select {
	case <-s.ready:
		return s.addr, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Run implements contracts/app.Runner: serve until ctx cancels, then
// flip health to NOT_SERVING, drain gracefully within ShutdownGrace,
// and hard-stop whatever remains.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.addr = ln.Addr().String()
	s.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	close(s.ready)

	errc := make(chan error, 1)
	go func() { errc <- s.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	s.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	done := make(chan struct{})
	go func() { s.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(s.cfg.ShutdownGrace):
		s.Stop()
		<-done
	}
	return nil
}

// RecoverUnary converts handler panics into codes.Internal instead of
// killing the process. A nil logger means slog.Default().
func RecoverUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if v := recover(); v != nil {
				l := log
				if l == nil {
					l = slog.Default()
				}
				l.ErrorContext(ctx, "grpc panic recovered",
					"method", info.FullMethod, "error", v, "stack", string(debug.Stack()))
				err = status.Errorf(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// LogUnary emits one structured line per RPC: method, code, duration.
func LogUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		l := log
		if l == nil {
			l = slog.Default()
		}
		l.InfoContext(ctx, "grpc request",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"duration", time.Since(start))
		return resp, err
	}
}
