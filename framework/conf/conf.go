// Package conf implements the kernel configuration policy
// (contracts/config): layered precedence, dot-key access, typed reads and
// struct binding. Kernel-native sources: defaults, JSON files, .env files,
// process environment, explicit overrides. Other formats (YAML, TOML,
// remote stores) arrive as contracts/config.Source adapters.
//
// Precedence (kernel invariant, doc 06 §8):
//
//	defaults < files < .env < environment < explicit overrides
package conf

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mgo-framework/mgo/contracts/config"
)

// Loader assembles configuration layers in precedence order.
type Loader struct {
	layers []config.Source
}

// NewLoader returns an empty loader. Add layers lowest-precedence first;
// the convenience methods encode the canonical order.
func NewLoader() *Loader { return &Loader{} }

// Add appends a source as the next (higher-precedence) layer.
func (l *Loader) Add(s config.Source) *Loader {
	l.layers = append(l.layers, s)
	return l
}

// Defaults adds a literal defaults tree (lowest precedence — call first).
func (l *Loader) Defaults(tree map[string]any) *Loader {
	return l.Add(mapSource(tree))
}

// JSONFile adds a JSON file layer. Missing files are skipped silently when
// optional is true.
func (l *Loader) JSONFile(path string, optional bool) *Loader {
	return l.Add(jsonFile{path: path, optional: optional})
}

// DotEnv adds a .env file layer (KEY=VALUE lines; # comments). Keys are
// mapped APP_NAME → app.name.
func (l *Loader) DotEnv(path string, optional bool) *Loader {
	return l.Add(dotEnvFile{path: path, optional: optional})
}

// Env adds the process environment as a layer, filtered by prefix
// (e.g. "MGO_"); MGO_APP_NAME → app.name.
func (l *Loader) Env(prefix string) *Loader {
	return l.Add(envSource{prefix: prefix})
}

// Overrides adds explicit highest-precedence values (flags, tests).
func (l *Loader) Overrides(values map[string]any) *Loader {
	return l.Add(mapSource(values))
}

// Load merges all layers into an immutable Config view.
func (l *Loader) Load() (config.Config, error) {
	merged := map[string]any{}
	for _, layer := range l.layers {
		tree, err := layer.Load()
		if err != nil {
			return nil, fmt.Errorf("conf: %w", err)
		}
		deepMerge(merged, tree)
	}
	return &view{tree: merged}, nil
}

// deepMerge merges src into dst; maps merge recursively, scalars/slices win.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
			cp := map[string]any{}
			deepMerge(cp, sm)
			dst[k] = cp
			continue
		}
		dst[k] = sv
	}
}

// ---- sources ----

type mapSource map[string]any

func (m mapSource) Load() (map[string]any, error) {
	// Expand dot keys ("app.name": x) into nested maps for uniform merging.
	out := map[string]any{}
	for k, v := range m {
		setPath(out, k, v)
	}
	return out, nil
}

type jsonFile struct {
	path     string
	optional bool
}

func (j jsonFile) Load() (map[string]any, error) {
	raw, err := os.ReadFile(j.path)
	if err != nil {
		if j.optional && os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("%s: %w", j.path, err)
	}
	return tree, nil
}

type dotEnvFile struct {
	path     string
	optional bool
}

func (d dotEnvFile) Load() (map[string]any, error) {
	raw, err := os.ReadFile(d.path)
	if err != nil {
		if d.optional && os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	out := map[string]any{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		setPath(out, envKeyToPath(strings.TrimSpace(key)), val)
	}
	return out, nil
}

type envSource struct{ prefix string }

func (e envSource) Load() (map[string]any, error) {
	out := map[string]any{}
	for _, kv := range os.Environ() {
		key, val, _ := strings.Cut(kv, "=")
		if e.prefix != "" {
			if !strings.HasPrefix(key, e.prefix) {
				continue
			}
			key = strings.TrimPrefix(key, e.prefix)
		}
		setPath(out, envKeyToPath(key), val)
	}
	return out, nil
}

// envKeyToPath maps APP_HTTP_PORT → app.http.port.
func envKeyToPath(key string) string {
	return strings.ToLower(strings.ReplaceAll(key, "_", "."))
}

// setPath writes value at a dot path inside tree, creating maps as needed.
func setPath(tree map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := tree
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = value
}

// ---- view ----

type view struct{ tree map[string]any }

func (v *view) lookup(path string) (any, bool) {
	if path == "" {
		return v.tree, true
	}
	var cur any = v.tree
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func (v *view) Has(path string) bool { _, ok := v.lookup(path); return ok }

func (v *view) Value(path string) any {
	val, _ := v.lookup(path)
	return val
}

func (v *view) String(path string, fallback ...string) string {
	if val, ok := v.lookup(path); ok {
		if s, ok := coerceString(val); ok {
			return s
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

func (v *view) Int(path string, fallback ...int) int {
	if val, ok := v.lookup(path); ok {
		if n, ok := coerceInt(val); ok {
			return n
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}

func (v *view) Bool(path string, fallback ...bool) bool {
	if val, ok := v.lookup(path); ok {
		if b, ok := coerceBool(val); ok {
			return b
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return false
}

func (v *view) Float(path string, fallback ...float64) float64 {
	if val, ok := v.lookup(path); ok {
		if f, ok := coerceFloat(val); ok {
			return f
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}

func (v *view) Sub(path string) config.Config {
	if val, ok := v.lookup(path); ok {
		if m, ok := val.(map[string]any); ok {
			return &view{tree: m}
		}
	}
	return &view{tree: map[string]any{}}
}

// Bind decodes the subtree at path into a struct pointer. Field mapping:
// `conf:"key"` tag, else lower-cased field name. Supports string, bool,
// int kinds, float kinds, time.Duration, slices of these, and nested
// structs. Unknown config keys are ignored; coercion failures error.
func (v *view) Bind(path string, target any) error {
	val, ok := v.lookup(path)
	if !ok {
		val = map[string]any{}
	}
	m, ok := val.(map[string]any)
	if !ok {
		return fmt.Errorf("conf: %q is not a section", path)
	}
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("conf: Bind target must be a struct pointer, got %T", target)
	}
	return bindStruct(m, rv.Elem(), path)
}

var durationType = reflect.TypeOf(time.Duration(0))

func bindStruct(m map[string]any, sv reflect.Value, at string) error {
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !f.IsExported() {
			continue
		}
		key := f.Tag.Get("conf")
		if key == "" {
			key = strings.ToLower(f.Name)
		}
		raw, ok := m[key]
		if !ok {
			continue
		}
		if err := bindValue(raw, sv.Field(i), at+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func bindValue(raw any, fv reflect.Value, at string) error {
	ft := fv.Type()
	// time.Duration accepts "5s" strings and numeric nanoseconds.
	if ft == durationType {
		switch x := raw.(type) {
		case string:
			d, err := time.ParseDuration(x)
			if err != nil {
				return fmt.Errorf("conf: %s: %w", at, err)
			}
			fv.SetInt(int64(d))
			return nil
		}
	}
	switch ft.Kind() {
	case reflect.String:
		s, ok := coerceString(raw)
		if !ok {
			return coerceErr(at, raw, "string")
		}
		fv.SetString(s)
	case reflect.Bool:
		b, ok := coerceBool(raw)
		if !ok {
			return coerceErr(at, raw, "bool")
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := coerceInt(raw)
		if !ok {
			return coerceErr(at, raw, "int")
		}
		fv.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := coerceInt(raw)
		if !ok || n < 0 {
			return coerceErr(at, raw, "uint")
		}
		fv.SetUint(uint64(n))
	case reflect.Float32, reflect.Float64:
		f, ok := coerceFloat(raw)
		if !ok {
			return coerceErr(at, raw, "float")
		}
		fv.SetFloat(f)
	case reflect.Struct:
		m, ok := raw.(map[string]any)
		if !ok {
			return coerceErr(at, raw, "section")
		}
		return bindStruct(m, fv, at)
	case reflect.Slice:
		items, ok := raw.([]any)
		if !ok {
			return coerceErr(at, raw, "list")
		}
		out := reflect.MakeSlice(ft, len(items), len(items))
		for i, item := range items {
			if err := bindValue(item, out.Index(i), fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
		fv.Set(out)
	case reflect.Map:
		m, ok := raw.(map[string]any)
		if !ok || ft.Key().Kind() != reflect.String {
			return coerceErr(at, raw, "map")
		}
		out := reflect.MakeMapWithSize(ft, len(m))
		for k, item := range m {
			ev := reflect.New(ft.Elem()).Elem()
			if err := bindValue(item, ev, at+"."+k); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k), ev)
		}
		fv.Set(out)
	default:
		return fmt.Errorf("conf: %s: unsupported field kind %s", at, ft.Kind())
	}
	return nil
}

func coerceErr(at string, raw any, want string) error {
	return fmt.Errorf("conf: %s: cannot use %T(%v) as %s", at, raw, raw, want)
}

// ---- coercions (env/dotenv values arrive as strings) ----

func coerceString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	}
	return "", false
}

func coerceInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n, true
		}
	}
	return 0, false
}

func coerceBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(x)); err == nil {
			return b, true
		}
	}
	return false, false
}

func coerceFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
