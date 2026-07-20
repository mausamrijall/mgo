// Package mgotest is MGO's first-party testing DSL — deliberately thin:
// an app harness with automatic shutdown, an HTTP request helper, and
// contract-level test doubles. It composes with the standard testing
// package (and therefore testify/ginkgo); it does not replace it. The
// Memory drivers in framework/cache and framework/queue are already the
// cache/queue fakes — use them directly.
package mgotest

import (
	"context"
	"testing"
	"time"

	"github.com/mgo-framework/mgo/contracts/config"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/mgo"
)

// Config builds a config from an override tree — the shortest way to a
// test configuration:
//
//	cfg := mgotest.Config(t, map[string]any{"http": map[string]any{"addr": ":0"}})
func Config(t testing.TB, tree map[string]any) config.Config {
	t.Helper()
	cfg, err := conf.NewLoader().Overrides(tree).Load()
	if err != nil {
		t.Fatalf("mgotest: config: %v", err)
	}
	return cfg
}

// TestApp is a booted application under test.
type TestApp struct {
	*mgo.App
	t testing.TB
}

// App builds and BOOTS an application (providers registered, container
// validated, Boot hooks run). Runners are not started — call Start for
// that. Boot failures fail the test immediately.
func App(t testing.TB, opts ...mgo.Option) *TestApp {
	t.Helper()
	app := mgo.New(opts...)
	if err := app.Boot(context.Background()); err != nil {
		t.Fatalf("mgotest: boot: %v", err)
	}
	return &TestApp{App: app, t: t}
}

// Start runs the app's runners in the background. Shutdown happens
// automatically at test cleanup; a run error fails the test.
func (a *TestApp) Start() *TestApp {
	a.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	a.t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				a.t.Errorf("mgotest: app run: %v", err)
			}
		case <-time.After(10 * time.Second):
			a.t.Error("mgotest: app did not shut down within 10s")
		}
	})
	return a
}
