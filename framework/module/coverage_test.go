package module_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	appc "github.com/mgo-framework/mgo/contracts/app"
	ormc "github.com/mgo-framework/mgo/contracts/orm"
	"github.com/mgo-framework/mgo/framework/module"
)

type failingMigrator struct{}

func (failingMigrator) Migrate(context.Context) error { return errors.New("ddl failed") }

type failMigModule struct{ crmModule }

func (failMigModule) ModuleName() string        { return "flaky" }
func (failMigModule) Migrations() ormc.Migrator { return failingMigrator{} }

func TestMigrateAllAttributesFailures(t *testing.T) {
	err := module.MigrateAll(context.Background(), failMigModule{})
	if err == nil || !strings.Contains(err.Error(), "module flaky") {
		t.Fatalf("err = %v, want module-attributed failure", err)
	}
}

func TestMigratorsIndexesByName(t *testing.T) {
	billing := &billingModule{}
	ms := module.Migrators(billing, crmModule{})
	if len(ms) != 1 {
		t.Fatalf("migrators = %d, want 1 (crm has none)", len(ms))
	}
	if _, ok := ms["billing"]; !ok {
		t.Fatal("billing migrator missing")
	}
}

type unnamedModule struct{ crmModule }

func (unnamedModule) ModuleName() string { return "" }

type nilMigModule struct{ crmModule }

func (nilMigModule) ModuleName() string        { return "nilmig" }
func (nilMigModule) Migrations() ormc.Migrator { return nil }

func TestCheckEdgeCases(t *testing.T) {
	if p := module.Check(unnamedModule{}); len(p) != 1 || p[0].String() != "(unnamed): ModuleName is empty" {
		t.Fatalf("unnamed = %v", p)
	}
	if p := module.Check(nilMigModule{}); len(p) != 1 || !strings.Contains(p[0].String(), "nil Migrations") {
		t.Fatalf("nil migrator = %v", p)
	}
	for name, wantOK := range map[string]bool{
		"billing": true, "a-b1": true,
		"-lead": false, "trail-": false, "UPPER": false, "under_score": false,
	} {
		mod := renamedModule{name: name}
		problems := module.Check(mod)
		if ok := len(problems) == 0; ok != wantOK {
			t.Fatalf("name %q: problems = %v, want ok=%v", name, problems, wantOK)
		}
	}
}

type renamedModule struct{ name string }

func (m renamedModule) ModuleName() string      { return m.name }
func (m renamedModule) Register(appc.App) error { return nil }
func (m renamedModule) Routes() http.Handler    { return http.NotFoundHandler() }
