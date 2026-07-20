// Package module defines MGO's module contract: a vertical slice of an
// application (billing, crm, ...) that owns its routes, config section,
// and migrations under one namespace. A module IS an app provider — the
// kernel needs nothing new to boot one; this contract only adds the
// namespacing glue reads.
//
// Capabilities follow MGO's usual pattern: optional interfaces
// discovered by type assertion. A module without HTTP routes simply
// doesn't implement HTTPModule.
package module

import (
	"net/http"

	"github.com/mgo-framework/mgo/contracts/app"
	"github.com/mgo-framework/mgo/contracts/config"
	"github.com/mgo-framework/mgo/contracts/orm"
)

// Module is a named vertical slice. ModuleName is the namespace for
// everything the module owns: its route prefix (/<name>), its config
// section (modules.<name>), its migration table isolation, its log/
// diagnostic labels. Names must be lowercase [a-z0-9-] and unique per
// app; framework/module.Check enforces this.
type Module interface {
	app.Provider
	ModuleName() string
}

// HTTPModule is the routes capability. Routes returns a handler serving
// the module's routes RELATIVE to its mount point — the module must not
// assume where it is mounted (the glue strips the prefix). This is what
// makes modules relocatable and isolation testable.
type HTTPModule interface {
	Module
	Routes() http.Handler
}

// MigratorModule is the migrations capability: the module's schema
// migrations as a contracts/orm.Migrator (goose, AutoMigrate, anything).
// The glue runs them namespaced during boot or `migrate`.
type MigratorModule interface {
	Module
	Migrations() orm.Migrator
}

// ConfigurableModule is the config capability: the module declares the
// struct it binds from its modules.<name> section, so tooling can
// validate configuration exists before boot. Bind still happens in
// Register/Boot via module.Config.
type ConfigurableModule interface {
	Module
	ConfigPrototype() any
}

// Config returns the module's own config view: the modules.<name>
// subtree. Modules read ONLY this — reaching into another module's
// section is the isolation violation the linter looks for.
func Config(cfg config.Config, m Module) config.Config {
	return cfg.Sub("modules." + m.ModuleName())
}
