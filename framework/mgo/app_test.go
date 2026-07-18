package mgo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
	"github.com/mgo-framework/mgo/framework/di"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
)

// recorder tracks lifecycle calls across providers.
type recorder struct{ events []string }

type provider struct {
	name    string
	rec     *recorder
	failMsg string // non-empty → Boot fails
}

func (p *provider) Register(app appc.App) error {
	p.rec.events = append(p.rec.events, "register:"+p.name)
	return nil
}
func (p *provider) Boot(ctx context.Context, app appc.App) error {
	if p.failMsg != "" {
		return errors.New(p.failMsg)
	}
	p.rec.events = append(p.rec.events, "boot:"+p.name)
	return nil
}
func (p *provider) Close(ctx context.Context) error {
	p.rec.events = append(p.rec.events, "close:"+p.name)
	return nil
}

func TestProviderOrdering(t *testing.T) {
	rec := &recorder{}
	app := mgo.New(mgo.WithProviders(
		&provider{name: "a", rec: rec},
		&provider{name: "b", rec: rec},
	))
	if err := app.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"register:a", "register:b", "boot:a", "boot:b"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", rec.events, want)
	}
}

func TestBootFailureUnwindsInReverseOrder(t *testing.T) {
	rec := &recorder{}
	app := mgo.New(mgo.WithProviders(
		&provider{name: "a", rec: rec},
		&provider{name: "b", rec: rec},
		&provider{name: "c", rec: rec, failMsg: "c exploded"},
	))
	err := app.Boot(context.Background())
	if err == nil || !errors.Is(err, err) || err.Error() == "" {
		t.Fatal("boot must fail")
	}
	// a and b booted, then closed in reverse (b before a).
	want := []string{"register:a", "register:b", "register:c", "boot:a", "boot:b", "close:b", "close:a"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", rec.events, want)
	}
}

func TestKernelBindingsAvailableToProviders(t *testing.T) {
	var sawConfig bool
	p := mgo.ProviderFunc(func(app appc.App) error {
		sawConfig = app.Config() != nil
		return di.Singleton[io.Writer](app.Container(), func() io.Writer { return io.Discard })
	})
	app := mgo.New(mgo.WithProviders(p))
	if err := app.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sawConfig {
		t.Fatal("provider did not see config")
	}
	if _, err := di.Make[io.Writer](app.Container()); err != nil {
		t.Fatal(err)
	}
}

func TestBootIsIdempotent(t *testing.T) {
	rec := &recorder{}
	app := mgo.New(mgo.WithProviders(&provider{name: "a", rec: rec}))
	ctx := context.Background()
	if err := app.Boot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.Boot(ctx); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) != 2 { // register + boot exactly once
		t.Fatalf("events = %v", rec.events)
	}
}

// blockingRunner runs until cancelled; records the fact it stopped cleanly.
type blockingRunner struct {
	stopped atomic.Bool
}

func (r *blockingRunner) Name() string { return "blocker" }
func (r *blockingRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	r.stopped.Store(true)
	return nil
}

// failingRunner fails immediately with a fatal error.
type failingRunner struct{}

func (failingRunner) Name() string              { return "failer" }
func (failingRunner) Run(context.Context) error { return errors.New("fatal runner error") }

func TestRunnerFailureStopsApp(t *testing.T) {
	blocker := &blockingRunner{}
	app := mgo.New(mgo.WithShutdownTimeout(2 * time.Second))
	app.AddRunner(blocker)
	app.AddRunner(failingRunner{})

	done := make(chan error, 1)
	go func() { done <- app.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil || err.Error() != "runner failer: fatal runner error" {
			t.Fatalf("want fatal runner error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not stop after runner failure")
	}
	if !blocker.stopped.Load() {
		t.Fatal("other runner was not cancelled")
	}
}

func TestGracefulShutdownViaContextCancel(t *testing.T) {
	rec := &recorder{}
	blocker := &blockingRunner{}
	app := mgo.New(
		mgo.WithProviders(&provider{name: "a", rec: rec}),
		mgo.WithShutdownTimeout(2*time.Second),
	)
	app.AddRunner(blocker)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	time.Sleep(50 * time.Millisecond) // let it start
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("clean shutdown returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not shut down")
	}
	if !blocker.stopped.Load() {
		t.Fatal("runner not drained")
	}
	// Provider closed during shutdown.
	if fmt.Sprint(rec.events[len(rec.events)-1]) != "close:a" {
		t.Fatalf("provider not closed: %v", rec.events)
	}
}

// TestHelloAppEndToEnd is the Phase 1 exit criterion: a hello handler
// served via stdlib mux through the full app lifecycle with graceful stop.
func TestHelloAppEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello from mgo")
	})
	runner := httpserver.New("http", mux, httpserver.Config{Addr: "127.0.0.1:0"})

	app := mgo.New(mgo.WithShutdownTimeout(2 * time.Second))
	app.AddRunner(runner)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	addrCtx, addrCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer addrCancel()
	addr, err := runner.Addr(addrCtx)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get("http://" + addr + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "hello from mgo" {
		t.Fatalf("got %d %q", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no graceful stop")
	}
}
