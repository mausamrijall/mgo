package httpserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/framework/httpserver"
)

func TestRunnerServesAndReportsAddr(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	r := httpserver.New("http", h, httpserver.Config{Addr: "127.0.0.1:0"})
	if r.Name() != "http" {
		t.Fatalf("name = %q", r.Name())
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	addrCtx, addrCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer addrCancel()
	addr, err := r.Addr(addrCtx)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
}

func TestAddrHonorsContext(t *testing.T) {
	r := httpserver.New("http", http.NotFoundHandler(), httpserver.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Addr(ctx); err == nil {
		t.Fatal("Addr on a never-started runner must respect ctx")
	}
}

func TestRunListenError(t *testing.T) {
	// Occupy a port, then ask a second runner for the same one.
	first := httpserver.New("a", http.NotFoundHandler(), httpserver.Config{Addr: "127.0.0.1:0"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go first.Run(ctx)
	addr, err := first.Addr(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	second := httpserver.New("b", http.NotFoundHandler(), httpserver.Config{Addr: addr})
	if err := second.Run(context.Background()); err == nil {
		t.Fatal("expected listen error on occupied port")
	}
}
