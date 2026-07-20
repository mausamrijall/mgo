package main

// The Phase 11 exit: ONE integration suite, run against the monolith
// wiring and the extracted-gRPC wiring. The suite cannot tell them
// apart — including the domain error path crossing the wire.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mgogrpc "github.com/mgo-framework/mgo/adapters/grpc-server"
	greeterv1 "github.com/mgo-framework/mgo/examples/greeter/gen/greeter/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// greeterSuite is the unchanged integration suite.
func greeterSuite(t *testing.T, base string) {
	t.Helper()

	resp, err := http.Get(base + "/greet/gopher")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /greet/gopher = %d, want 200", resp.StatusCode)
	}
	var body struct{ Greeting string }
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Greeting != "Hello, gopher!" {
		t.Fatalf("greeting = %q", body.Greeting)
	}

	// Domain error: empty name → 400, identically on every transport.
	resp2, err := http.Get(base + "/greet/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /greet/ = %d, want 400", resp2.StatusCode)
	}
}

func TestMonolithTransport(t *testing.T) {
	srv := httptest.NewServer(buildHTTP(localGreeter{}))
	defer srv.Close()
	greeterSuite(t, srv.URL)
}

func TestExtractedGRPCTransport(t *testing.T) {
	// The extracted service: gRPC server hosting the SAME implementation.
	gsrv := mgogrpc.New(mgogrpc.Config{Addr: "127.0.0.1:0", ShutdownGrace: 2 * time.Second})
	greeterv1.RegisterGreeterServiceServer(gsrv, grpcService{impl: localGreeter{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- gsrv.Run(ctx) }()

	addrCtx, addrCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer addrCancel()
	addr, err := gsrv.Addr(addrCtx)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// The app, unchanged, now consuming the wire.
	srv := httptest.NewServer(buildHTTP(grpcGreeter{client: greeterv1.NewGreeterServiceClient(conn)}))
	defer srv.Close()

	greeterSuite(t, srv.URL) // ← the exact same suite

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("grpc server did not stop")
	}
}
