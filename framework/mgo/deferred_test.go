package mgo_test

import (
	"context"
	"strings"
	"testing"

	appc "github.com/mgo-framework/mgo/contracts/app"
	"github.com/mgo-framework/mgo/framework/di"
	"github.com/mgo-framework/mgo/framework/mgo"
)

type Mailer interface{ Send(to string) error }

type smtpMailer struct{}

func (smtpMailer) Send(string) error { return nil }

// deferredMailProvider registers its binding only when Mailer is first
// resolved.
type deferredMailProvider struct{ registered *int }

func (p deferredMailProvider) Register(app appc.App) error {
	*p.registered++
	return di.Singleton[Mailer](app.Container(), func() Mailer { return smtpMailer{} })
}

func (p deferredMailProvider) Provides() []any { return []any{(*Mailer)(nil)} }

func TestDeferrableProviderRegistersOnFirstUse(t *testing.T) {
	registered := 0
	app := mgo.New(mgo.WithProviders(deferredMailProvider{registered: &registered}))
	if err := app.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registered != 0 {
		t.Fatal("deferred provider registered during boot")
	}
	m := di.MustMake[Mailer](app.Container())
	if err := m.Send("x"); err != nil {
		t.Fatal(err)
	}
	di.MustMake[Mailer](app.Container())
	if registered != 1 {
		t.Fatalf("registered %d times, want 1", registered)
	}
}

type badDeferred struct{ deferredMailProvider }

func (badDeferred) Boot(context.Context, appc.App) error { return nil }

func TestDeferrableCannotBeBootable(t *testing.T) {
	registered := 0
	app := mgo.New(mgo.WithProviders(badDeferred{deferredMailProvider{registered: &registered}}))
	err := app.Boot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Deferrable and Bootable") {
		t.Fatalf("want Deferrable+Bootable rejection, got %v", err)
	}
}
