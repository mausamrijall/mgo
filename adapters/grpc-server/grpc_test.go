package mgogrpc_test

// The health service ships precompiled with grpc-go, so lifecycle tests
// need no codegen: dial, health-check, cancel, verify graceful stop.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	mgogrpc "github.com/mgo-framework/mgo/adapters/grpc-server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func startServer(t *testing.T) (*mgogrpc.Server, context.CancelFunc, chan error) {
	t.Helper()
	s := mgogrpc.New(mgogrpc.Config{Addr: "127.0.0.1:0", ShutdownGrace: 2 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(cancel)
	return s, cancel, done
}

func dial(t *testing.T, s *mgogrpc.Server) *grpc.ClientConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addr, err := s.Addr(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestServesHealthAndStopsGracefully(t *testing.T) {
	s, cancel, done := startServer(t)
	conn := dial(t, s)

	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health = %v, want SERVING", resp.Status)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after cancel")
	}
}

func TestRecoverUnaryConvertsPanics(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(slog.Default())

	interceptor := mgogrpc.RecoverUnary(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Boom"},
		func(ctx context.Context, req any) (any, error) { panic("kaboom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("panic mapped to %v, want Internal", status.Code(err))
	}
}

func TestLogUnaryPassesThrough(t *testing.T) {
	interceptor := mgogrpc.LogUnary(slog.New(slog.NewTextHandler(io.Discard, nil)))
	want := errors.New("boom")
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(ctx context.Context, req any) (any, error) { return "ok", want })
	if resp != "ok" || !errors.Is(err, want) {
		t.Fatalf("interceptor altered result: %v %v", resp, err)
	}
}
