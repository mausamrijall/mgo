// Package module is the glue that assembles contracts/module slices into
// an application: prefix-mounted routes, aggregated migrations, and the
// Check linter that keeps modules honest about their namespaces.
package module

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	appc "github.com/mgo-framework/mgo/contracts/app"
	modulec "github.com/mgo-framework/mgo/contracts/module"
	ormc "github.com/mgo-framework/mgo/contracts/orm"
	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Providers converts modules to app providers for mgo.WithProviders —
// registration order is module order.
func Providers(mods ...modulec.Module) []appc.Provider {
	out := make([]appc.Provider, len(mods))
	for i, m := range mods {
		out[i] = m
	}
	return out
}

// MountAll mounts every HTTPModule under /<name>/ on the router. The
// module's handler sees paths RELATIVE to its mount (the prefix is
// stripped), so a module cannot know or care where it lives — and two
// modules cannot collide, because each owns exactly its /<name>/ subtree.
func MountAll(r routerc.Router, mods ...modulec.Module) error {
	seen := map[string]bool{}
	for _, m := range mods {
		name := m.ModuleName()
		if seen[name] {
			return fmt.Errorf("module: duplicate module name %q", name)
		}
		seen[name] = true
		hm, ok := m.(modulec.HTTPModule)
		if !ok {
			continue
		}
		prefix := "/" + name
		r.Mount(prefix, http.StripPrefix(prefix, hm.Routes()))
	}
	return nil
}

// MigrateAll runs every MigratorModule's migrations in module order,
// failing with the module's name attached.
func MigrateAll(ctx context.Context, mods ...modulec.Module) error {
	for _, m := range mods {
		mm, ok := m.(modulec.MigratorModule)
		if !ok {
			continue
		}
		if err := mm.Migrations().Migrate(ctx); err != nil {
			return fmt.Errorf("module %s: migrate: %w", m.ModuleName(), err)
		}
	}
	return nil
}

// Migrators exposes each module's migrator keyed by name — for `mgo
// migrate`-style tooling that wants per-module control.
func Migrators(mods ...modulec.Module) map[string]ormc.Migrator {
	out := map[string]ormc.Migrator{}
	for _, m := range mods {
		if mm, ok := m.(modulec.MigratorModule); ok {
			out[m.ModuleName()] = mm.Migrations()
		}
	}
	return out
}

// Problem is one linter finding.
type Problem struct {
	Module string
	Issue  string
}

func (p Problem) String() string { return p.Module + ": " + p.Issue }

// Check lints one module: name validity and capability sanity. It is a
// library function so tests and (post-freeze) a CLI command share it.
func Check(m modulec.Module) []Problem {
	var out []Problem
	name := m.ModuleName()

	if name == "" {
		out = append(out, Problem{Module: "(unnamed)", Issue: "ModuleName is empty"})
		return out
	}
	if !validName(name) {
		out = append(out, Problem{Module: name, Issue: "name must be lowercase [a-z0-9-], no leading/trailing dash"})
	}
	if hm, ok := m.(modulec.HTTPModule); ok {
		if hm.Routes() == nil {
			out = append(out, Problem{Module: name, Issue: "HTTPModule with nil Routes()"})
		}
	}
	if mm, ok := m.(modulec.MigratorModule); ok {
		if mm.Migrations() == nil {
			out = append(out, Problem{Module: name, Issue: "MigratorModule with nil Migrations()"})
		}
	}
	return out
}

// CheckAll lints a set of modules plus cross-module rules (duplicate
// names). Empty result = clean.
func CheckAll(mods ...modulec.Module) []Problem {
	var out []Problem
	seen := map[string]bool{}
	for _, m := range mods {
		out = append(out, Check(m)...)
		name := m.ModuleName()
		if seen[name] {
			out = append(out, Problem{Module: name, Issue: "duplicate module name"})
		}
		seen[name] = true
	}
	return out
}

func validName(name string) bool {
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
