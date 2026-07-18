package httpserver_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
)

// TestProvider boots a full app through httpserver.Provider and serves a
// request — the one-line UseRouter path, end to end.
func TestProvider(t *testing.T) {
	// Reserve a free port; tiny race window between Close and the runner's
	// Listen is acceptable in tests.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg, err := conf.NewLoader().
		Overrides(map[string]any{"http": map[string]any{"addr": addr}}).
		Load()
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	})

	app := mgo.New(
		mgo.WithConfig(cfg),
		mgo.WithProviders(httpserver.Provider("http", mux)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/ping")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if body != "pong" {
		t.Fatalf("body = %q, want pong", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not shut down")
	}
}
