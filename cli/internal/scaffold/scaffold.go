// Package scaffold renders new MGO applications and their per-axis files.
// Render is a pure function (options in, files out), which makes every
// stack combination snapshot-testable and lets swap regenerate exactly
// the files an axis owns — nothing else.
package scaffold

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templates embed.FS

// Axes and their allowed values. The wizard, flag validation, and swap
// all read from this single table.
var Axes = map[string][]string{
	"router": {"chi", "stdmux"},
	"db":     {"none", "gorm", "sql"},
}

// Options selects one point in the stack matrix.
type Options struct {
	Name   string // directory + display name
	Module string // go module path
	Router string // chi | stdmux
	DB     string // none | gorm | sql
	MGOSrc string // path to the MGO source tree (replace target)
}

// HasDB reports whether a database axis is active (template helper).
func (o Options) HasDB() bool { return o.DB != "" && o.DB != "none" }

// Validate checks axis values.
func (o Options) Validate() error {
	if o.Name == "" {
		return fmt.Errorf("project name is required")
	}
	for axis, val := range map[string]string{"router": o.Router, "db": o.DB} {
		if !contains(Axes[axis], val) {
			return fmt.Errorf("invalid %s %q (want one of: %s)", axis, val, strings.Join(Axes[axis], ", "))
		}
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Owned maps each axis to the files it owns. Swap regenerates exactly
// these; everything else in the project is the user's.
var Owned = map[string][]string{
	"router": {"router.go"},
	"db":     {"store.go", "main.go"}, // main.go wires the store, so db owns it
}

// Render produces the full file set for a new project (path → contents).
func Render(o Options) (map[string]string, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	files := map[string]string{}
	plan := []struct{ out, tmpl string }{
		{"go.mod", "go.mod.tmpl"},
		{"main.go", "main.go.tmpl"},
		{"handlers.go", "handlers.go.tmpl"},
		{"main_test.go", "main_test.go.tmpl"},
		{"router.go", "router_" + o.Router + ".tmpl"},
		{".env.example", "env.tmpl"},
		{".gitignore", "gitignore.tmpl"},
		{"README.md", "readme.tmpl"},
	}
	if o.HasDB() {
		plan = append(plan, struct{ out, tmpl string }{"store.go", "store_" + o.DB + ".tmpl"})
	}
	for _, p := range plan {
		s, err := render(p.tmpl, o)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", p.out, err)
		}
		files[p.out] = s
	}
	return files, nil
}

// RenderOwned renders only the files owned by axis, for swap.
func RenderOwned(o Options, axis string) (map[string]string, error) {
	all, err := Render(o)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range Owned[axis] {
		if f == "store.go" && !o.HasDB() {
			continue // db=none has no store; swap deletes it instead
		}
		out[f] = all[f]
	}
	return out, nil
}

func render(name string, o Options) (string, error) {
	t, err := template.ParseFS(templates, "templates/"+name)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, o); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Replaces lists the go.mod replace directives a project needs (template
// and swap both use it — one source of truth).
func (o Options) Replaces() []struct{ Mod, Path string } {
	mgo := func(m string) struct{ Mod, Path string } {
		return struct{ Mod, Path string }{
			Mod:  "github.com/mgo-framework/mgo/" + m,
			Path: filepath.Join(o.MGOSrc, filepath.FromSlash(m)),
		}
	}
	out := []struct{ Mod, Path string }{mgo("contracts"), mgo("framework")}
	out = append(out, mgo("adapters/router-"+o.Router))
	switch o.DB {
	case "gorm":
		out = append(out, mgo("adapters/orm-gorm"))
	case "sql":
		out = append(out, mgo("adapters/db-sql"))
	}
	return out
}

// ---- manifest ----

// ManifestName is the project manifest file written by `mgo new`.
const ManifestName = "mgo.json"

// Manifest records the project's stack choices and the hashes of
// generated files, so swap can detect user edits before overwriting.
type Manifest struct {
	Name      string            `json:"name"`
	Module    string            `json:"module"`
	Router    string            `json:"router"`
	DB        string            `json:"db"`
	MGOSrc    string            `json:"mgo_src"`
	Generated map[string]string `json:"generated"` // path → sha256
}

// Hash returns the manifest hash for file contents.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Load reads the manifest in dir.
func Load(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("no %s here — run this inside an mgo project (or `mgo new` one): %w", ManifestName, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ManifestName, err)
	}
	if m.Generated == nil {
		m.Generated = map[string]string{}
	}
	return &m, nil
}

// Save writes the manifest to dir with stable key order.
func (m *Manifest) Save(dir string) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestName), append(raw, '\n'), 0o644)
}

// Options rebuilds scaffold options from the manifest.
func (m *Manifest) Options() Options {
	return Options{Name: m.Name, Module: m.Module, Router: m.Router, DB: m.DB, MGOSrc: m.MGOSrc}
}

// Modified returns the generated files whose on-disk content no longer
// matches the recorded hash (i.e. the user edited them), limited to the
// given set (or all recorded files if paths is empty).
func (m *Manifest) Modified(dir string, paths ...string) ([]string, error) {
	check := paths
	if len(check) == 0 {
		for p := range m.Generated {
			check = append(check, p)
		}
	}
	sort.Strings(check)
	var out []string
	for _, p := range check {
		want, tracked := m.Generated[p]
		if !tracked {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, p))
		if os.IsNotExist(err) {
			continue // deleted by user: treat as taken over
		}
		if err != nil {
			return nil, err
		}
		if Hash(string(raw)) != want {
			out = append(out, p)
		}
	}
	return out, nil
}

// FileState classifies a tracked generated file against its recorded hash.
type FileState string

const (
	Unchanged FileState = "unchanged" // safe to regenerate
	Modified  FileState = "modified"  // user-edited; mgo will not touch it
	Deleted   FileState = "deleted"   // removed by the user; taken over
)

// Status reports the state of every tracked generated file, sorted by path.
func (m *Manifest) Status(dir string) ([]struct {
	Path  string
	State FileState
}, error) {
	paths := make([]string, 0, len(m.Generated))
	for p := range m.Generated {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]struct {
		Path  string
		State FileState
	}, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(filepath.Join(dir, p))
		state := Unchanged
		switch {
		case os.IsNotExist(err):
			state = Deleted
		case err != nil:
			return nil, err
		case Hash(string(raw)) != m.Generated[p]:
			state = Modified
		}
		out = append(out, struct {
			Path  string
			State FileState
		}{p, state})
	}
	return out, nil
}

// untracked files are written but never hash-tracked: go tooling rewrites
// go.mod legitimately (tidy, go get), and mgo evolves it with `go mod
// edit` rather than regeneration — so "modified" would be noise.
var untracked = map[string]bool{"go.mod": true}

// WriteFiles writes rendered files under dir and records their hashes.
func WriteFiles(dir string, files map[string]string, m *Manifest) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(files[p]), 0o644); err != nil {
			return err
		}
		if m != nil && !untracked[p] {
			m.Generated[p] = Hash(files[p])
		}
	}
	return nil
}
