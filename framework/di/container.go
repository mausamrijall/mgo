// Package di implements the MGO service container (contracts/container).
//
// Design (docs/architecture/02 §6, hardened in Phase 3): reflection happens
// at registration and boot only. Validate() checks the whole graph, then
// compiles each binding's parameter list into direct binding references and
// pre-boxes instances. After boot, resolution is map lookups, mutexes, and
// function calls: singleton and scoped hits are zero-allocation, and
// bindings registered through the typed FuncBinder path never touch
// reflect at all — that path is the target shape for `mgo --di=codegen`.
package di

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mgo-framework/mgo/contracts/container"
)

var errType = reflect.TypeOf((*error)(nil)).Elem()

// binding is a registered constructor with its pre-computed plan.
type binding struct {
	key      reflect.Type
	lifetime container.Lifetime

	// Reflective constructor path.
	ctor   reflect.Value
	params []reflect.Type
	hasErr bool

	// Typed constructor path (FuncBinder): bypasses reflect entirely.
	fastCtor func(container.Resolver) (any, error)

	// deps is compiled at Validate (and after deferred materialization):
	// deps[i] resolves params[i]. A nil entry falls back to dynamic
	// resolution (deferred keys, late auto-bindings).
	deps []*binding

	once     sync.Once // singleton memoization
	boxed    any       // pre-boxed instance (singletons and Instance values)
	buildErr error
}

// deferredGroup is a lazy registration unit: register runs single-flight
// the first time one of its keys is resolved.
type deferredGroup struct {
	keys     []reflect.Type
	register func() error
	once     sync.Once
	err      error
}

// Container is the root implementation of contracts/container.Container.
type Container struct {
	mu         sync.RWMutex
	bindings   map[reflect.Type]*binding
	contextual map[reflect.Type]map[reflect.Type]*binding // consumer → param → binding
	deferred   map[reflect.Type]*deferredGroup
	sealed     bool
	unsealed   int // >0 permits binding post-seal (deferred materialization)

	compiled atomic.Bool
}

var (
	_ container.Container        = (*Container)(nil)
	_ container.ContextualBinder = (*Container)(nil)
	_ container.FuncBinder       = (*Container)(nil)
	_ container.Deferrer         = (*Container)(nil)
)

// New creates an empty root container.
func New() *Container {
	return &Container{
		bindings:   map[reflect.Type]*binding{},
		contextual: map[reflect.Type]map[reflect.Type]*binding{},
		deferred:   map[reflect.Type]*deferredGroup{},
	}
}

// keyType normalizes a binding key: (*T)(nil) → T, where T may itself be an
// interface or a concrete type (including pointer types).
func keyType(key any) (reflect.Type, error) {
	t := reflect.TypeOf(key)
	if t == nil || t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("di: key must be a typed nil pointer like (*T)(nil), got %T", key)
	}
	return t.Elem(), nil
}

// Bind implements container.Container.
func (c *Container) Bind(key any, constructor any, lifetime container.Lifetime) error {
	kt, err := keyType(key)
	if err != nil {
		return err
	}
	b, err := newBinding(kt, constructor, lifetime)
	if err != nil {
		return err
	}
	return c.put(kt, b)
}

// BindFunc implements container.FuncBinder: a typed constructor that
// resolves its own dependencies through the Resolver — no reflection.
func (c *Container) BindFunc(key any, fn func(container.Resolver) (any, error), lifetime container.Lifetime) error {
	kt, err := keyType(key)
	if err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("di: nil constructor func for %s", typeName(kt))
	}
	return c.put(kt, &binding{key: kt, lifetime: lifetime, fastCtor: fn})
}

// BindFor implements container.ContextualBinder.
func (c *Container) BindFor(consumer any, key any, constructor any, lifetime container.Lifetime) error {
	ct, err := keyType(consumer)
	if err != nil {
		return err
	}
	kt, err := keyType(key)
	if err != nil {
		return err
	}
	b, err := newBinding(kt, constructor, lifetime)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed && c.unsealed == 0 {
		return fmt.Errorf("di: container is sealed (Validate ran); cannot bind %s for %s", typeName(kt), typeName(ct))
	}
	m := c.contextual[ct]
	if m == nil {
		m = map[reflect.Type]*binding{}
		c.contextual[ct] = m
	}
	m[kt] = b
	return nil
}

// Defer implements container.Deferrer: register runs the first time any of
// keys is resolved. Deferred keys pass Validate's missing-binding check;
// their subgraph is validated when materialized.
func (c *Container) Defer(register func() error, keys ...any) error {
	if register == nil {
		return fmt.Errorf("di: Defer requires a register function")
	}
	if len(keys) == 0 {
		return fmt.Errorf("di: Defer requires at least one key")
	}
	g := &deferredGroup{register: register}
	for _, k := range keys {
		kt, err := keyType(k)
		if err != nil {
			return err
		}
		g.keys = append(g.keys, kt)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return fmt.Errorf("di: container is sealed; cannot defer registrations")
	}
	for _, kt := range g.keys {
		if _, exists := c.bindings[kt]; exists {
			return fmt.Errorf("di: deferred key %s is already bound", typeName(kt))
		}
		c.deferred[kt] = g
	}
	return nil
}

// newBinding validates a reflective constructor and builds its binding.
func newBinding(kt reflect.Type, constructor any, lifetime container.Lifetime) (*binding, error) {
	cv := reflect.ValueOf(constructor)
	ct := cv.Type()
	if !cv.IsValid() || ct.Kind() != reflect.Func {
		return nil, fmt.Errorf("di: constructor for %s must be a function, got %T", typeName(kt), constructor)
	}
	if ct.NumOut() == 0 || ct.NumOut() > 2 {
		return nil, fmt.Errorf("di: constructor for %s must return (T) or (T, error)", typeName(kt))
	}
	hasErr := ct.NumOut() == 2
	if hasErr && ct.Out(1) != errType {
		return nil, fmt.Errorf("di: constructor for %s second return must be error, got %s", typeName(kt), ct.Out(1))
	}
	if out := ct.Out(0); !out.AssignableTo(kt) {
		return nil, fmt.Errorf("di: constructor for %s returns %s which is not assignable to it", typeName(kt), typeName(out))
	}
	params := make([]reflect.Type, ct.NumIn())
	for i := range params {
		params[i] = ct.In(i)
	}
	return &binding{key: kt, lifetime: lifetime, ctor: cv, params: params, hasErr: hasErr}, nil
}

func (c *Container) put(kt reflect.Type, b *binding) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed && c.unsealed == 0 {
		return fmt.Errorf("di: container is sealed (Validate ran); cannot bind %s", typeName(kt))
	}
	c.bindings[kt] = b
	return nil
}

// Instance implements container.Container.
func (c *Container) Instance(key any, value any) error {
	kt, err := keyType(key)
	if err != nil {
		return err
	}
	vv := reflect.ValueOf(value)
	if !vv.IsValid() || !vv.Type().AssignableTo(kt) {
		if kt.Kind() == reflect.Interface && value == nil {
			return fmt.Errorf("di: nil instance for %s", typeName(kt))
		}
		if !vv.IsValid() {
			return fmt.Errorf("di: nil instance for %s", typeName(kt))
		}
		return fmt.Errorf("di: instance value %T is not assignable to %s", value, typeName(kt))
	}
	b := &binding{key: kt, lifetime: container.Singleton, boxed: value}
	b.once.Do(func() {}) // already built
	return c.put(kt, b)
}

func (c *Container) lookup(t reflect.Type) (*binding, bool) {
	c.mu.RLock()
	b, ok := c.bindings[t]
	c.mu.RUnlock()
	return b, ok
}

func (c *Container) contextualFor(consumer reflect.Type, param reflect.Type) *binding {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if m := c.contextual[consumer]; m != nil {
		return m[param]
	}
	return nil
}

// Resolve implements container.Resolver on the root: Scoped bindings error.
func (c *Container) Resolve(key any) (any, error) {
	kt, err := keyType(key)
	if err != nil {
		return nil, err
	}
	return c.resolveType(kt, nil, nil)
}

// resolveType resolves t. scope is nil for root resolution; path carries
// the chain for diagnostics and cycle detection before the graph is
// compiled (afterwards the graph is known acyclic and path stays nil).
func (c *Container) resolveType(t reflect.Type, scope *scopeImpl, path []reflect.Type) (any, error) {
	if !c.compiled.Load() {
		for _, p := range path {
			if p == t {
				return nil, fmt.Errorf("di: dependency cycle: %s", chain(path, t))
			}
		}
	}

	b, ok := c.lookup(t)
	if !ok {
		if g := c.deferredFor(t); g != nil {
			if err := c.materialize(g); err != nil {
				return nil, err
			}
			b, ok = c.lookup(t)
		}
	}
	if !ok {
		ab, err := c.autoBinding(t)
		if err != nil {
			hint := ""
			if t.Kind() == reflect.Interface {
				hint = " (interfaces require an explicit binding)"
			}
			return nil, fmt.Errorf("di: no binding for %s%s; chain: %s", typeName(t), hint, chain(path, t))
		}
		b = ab
	}
	return c.resolveBinding(b, scope, path)
}

func (c *Container) resolveBinding(b *binding, scope *scopeImpl, path []reflect.Type) (any, error) {
	switch b.lifetime {
	case container.Singleton:
		b.once.Do(func() {
			b.boxed, b.buildErr = c.construct(b, nil, append(path, b.key))
		})
		return b.boxed, b.buildErr
	case container.Scoped:
		if scope == nil {
			return nil, fmt.Errorf("di: %s is Scoped and cannot be resolved from the root container; chain: %s", typeName(b.key), chain(path, b.key))
		}
		return scope.instanceOf(b, path)
	default: // Transient
		return c.construct(b, scope, path)
	}
}

// construct invokes a binding's constructor, resolving parameters through
// the compiled plan when available.
func (c *Container) construct(b *binding, scope *scopeImpl, path []reflect.Type) (any, error) {
	if b.fastCtor != nil {
		var r container.Resolver = c
		if scope != nil {
			r = scope
		}
		return b.fastCtor(r)
	}
	args := make([]reflect.Value, len(b.params))
	for i, pt := range b.params {
		var dep any
		var err error
		switch {
		case b.deps != nil && b.deps[i] != nil:
			dep, err = c.resolveBinding(b.deps[i], scope, path)
		default:
			if cb := c.contextualFor(b.key, pt); cb != nil {
				dep, err = c.resolveBinding(cb, scope, append(path, b.key))
			} else {
				dep, err = c.resolveType(pt, scope, append(path, b.key))
			}
		}
		if err != nil {
			return nil, err
		}
		if dep == nil {
			args[i] = reflect.Zero(pt)
		} else {
			args[i] = reflect.ValueOf(dep)
		}
	}
	out := b.ctor.Call(args)
	if b.hasErr && !out[1].IsNil() {
		return nil, fmt.Errorf("di: constructor for %s failed: %w", typeName(b.key), out[1].Interface().(error))
	}
	return out[0].Interface(), nil
}

// autoBinding builds a transient binding for an unregistered concrete
// struct or pointer-to-struct type. Constructor injection only — no field
// injection (doc 02 §6.4).
func (c *Container) autoBinding(t reflect.Type) (*binding, error) {
	base := t
	if base.Kind() == reflect.Ptr {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return nil, fmt.Errorf("di: cannot auto-resolve %s", typeName(t))
	}
	return &binding{key: t, lifetime: container.Transient, fastCtor: func(container.Resolver) (any, error) {
		if t.Kind() == reflect.Ptr {
			return reflect.New(t.Elem()).Interface(), nil
		}
		return reflect.New(t).Elem().Interface(), nil
	}}, nil
}

// deferredFor returns the deferred group registered for t, if any.
func (c *Container) deferredFor(t reflect.Type) *deferredGroup {
	c.mu.RLock()
	g := c.deferred[t]
	c.mu.RUnlock()
	return g
}

// materialize runs a deferred group's registration single-flight, then
// validates and compiles the newly registered subgraph.
func (c *Container) materialize(g *deferredGroup) error {
	g.once.Do(func() {
		c.mu.Lock()
		c.unsealed++
		c.mu.Unlock()
		err := g.register()
		c.mu.Lock()
		c.unsealed--
		c.mu.Unlock()
		if err != nil {
			g.err = fmt.Errorf("di: deferred registration failed: %w", err)
			return
		}
		for _, kt := range g.keys {
			if _, ok := c.lookup(kt); !ok {
				g.err = fmt.Errorf("di: deferred provider promised %s but did not bind it", typeName(kt))
				return
			}
			if err := c.checkType(kt, nil, map[*binding]bool{}); err != nil {
				g.err = fmt.Errorf("di: deferred graph: %s", err)
				return
			}
		}
		if c.compiled.Load() {
			c.compile()
		}
	})
	return g.err
}

// Call implements container.Resolver.Call on the root container.
func (c *Container) Call(fn any) ([]any, error) {
	return call(c, nil, fn)
}

func call(c *Container, scope *scopeImpl, fn any) ([]any, error) {
	fv := reflect.ValueOf(fn)
	if !fv.IsValid() || fv.Type().Kind() != reflect.Func {
		return nil, fmt.Errorf("di: Call target must be a function, got %T", fn)
	}
	ft := fv.Type()
	args := make([]reflect.Value, ft.NumIn())
	for i := range args {
		v, err := c.resolveType(ft.In(i), scope, nil)
		if err != nil {
			return nil, err
		}
		if v == nil {
			args[i] = reflect.Zero(ft.In(i))
		} else {
			args[i] = reflect.ValueOf(v)
		}
	}
	out := fv.Call(args)
	res := make([]any, 0, len(out))
	for _, o := range out {
		res = append(res, o.Interface())
	}
	// Surface a trailing error return as the call error.
	if n := len(out); n > 0 && ft.Out(n-1) == errType && !out[n-1].IsNil() {
		return res[:n-1], out[n-1].Interface().(error)
	}
	return res, nil
}

// Validate implements container.Container: full-graph boot-time check,
// then plan compilation. After Validate, resolution performs no cycle
// detection and no per-parameter type lookups.
func (c *Container) Validate() error {
	c.mu.Lock()
	keys := make([]reflect.Type, 0, len(c.bindings))
	for k := range c.bindings {
		keys = append(keys, k)
	}
	c.sealed = true
	c.mu.Unlock()

	sort.Slice(keys, func(i, j int) bool { return typeName(keys[i]) < typeName(keys[j]) })

	var problems []string
	for _, k := range keys {
		if err := c.checkType(k, nil, map[*binding]bool{}); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("di: graph validation failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	c.compile()
	return nil
}

// checkType walks the dependency graph from key type k without
// constructing anything.
func (c *Container) checkType(k reflect.Type, path []reflect.Type, visiting map[*binding]bool) error {
	b, ok := c.lookup(k)
	if !ok {
		if _, deferred := c.deferredIndex(k); deferred {
			return nil // validated when materialized
		}
		if _, err := c.autoBinding(k); err != nil {
			hint := ""
			if k.Kind() == reflect.Interface {
				hint = " (interfaces require an explicit binding)"
			}
			return fmt.Errorf("no binding for %s%s; chain: %s", typeName(k), hint, chain(path, k))
		}
		return nil
	}
	return c.checkBinding(b, path, visiting)
}

func (c *Container) deferredIndex(k reflect.Type) (*deferredGroup, bool) {
	c.mu.RLock()
	g, ok := c.deferred[k]
	c.mu.RUnlock()
	return g, ok
}

// checkBinding validates one binding: cycles, lifetime rules, parameters.
func (c *Container) checkBinding(b *binding, path []reflect.Type, visiting map[*binding]bool) error {
	if visiting[b] {
		return fmt.Errorf("dependency cycle: %s", chain(path, b.key))
	}
	// Lifetime rule: nothing reached from a Singleton may be Scoped — the
	// scoped instance would escape its scope.
	if b.lifetime == container.Scoped {
		for _, anc := range path {
			if ab, _ := c.lookup(anc); ab != nil && ab.lifetime == container.Singleton {
				return fmt.Errorf("lifetime violation: singleton %s depends on scoped %s; chain: %s", typeName(anc), typeName(b.key), chain(path, b.key))
			}
		}
	}
	if b.fastCtor != nil {
		return nil // typed constructors resolve dynamically; nothing to walk
	}
	visiting[b] = true
	defer delete(visiting, b)
	for _, p := range b.params {
		if cb := c.contextualFor(b.key, p); cb != nil {
			if err := c.checkBinding(cb, append(path, b.key), visiting); err != nil {
				return err
			}
			continue
		}
		if err := c.checkType(p, append(path, b.key), visiting); err != nil {
			return err
		}
	}
	return nil
}

// compile precomputes each binding's parameter plan: direct binding
// references (contextual overrides applied), auto-bindings materialized
// into the map. Entries stay nil for deferred keys, which resolve
// dynamically until materialized.
func (c *Container) compile() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.bindings {
		c.compileLocked(b)
	}
	for _, m := range c.contextual {
		for _, b := range m {
			c.compileLocked(b)
		}
	}
	c.compiled.Store(true)
}

func (c *Container) compileLocked(b *binding) {
	if b.fastCtor != nil || len(b.params) == 0 {
		return // typed constructors and instances have nothing to plan
	}
	if b.deps != nil && !anyNil(b.deps) {
		return // already fully compiled
	}
	deps := make([]*binding, len(b.params))
	for i, pt := range b.params {
		if m := c.contextual[b.key]; m != nil && m[pt] != nil {
			deps[i] = m[pt]
			continue
		}
		if pb, ok := c.bindings[pt]; ok {
			deps[i] = pb
			continue
		}
		if _, deferred := c.deferred[pt]; deferred {
			continue // dynamic until materialized
		}
		if ab, err := c.autoBinding(pt); err == nil {
			c.bindings[pt] = ab
			deps[i] = ab
		}
	}
	b.deps = deps
}

func anyNil(deps []*binding) bool {
	for _, d := range deps {
		if d == nil {
			return true
		}
	}
	return false
}

// chain formats a resolution path for diagnostics.
func chain(path []reflect.Type, tail reflect.Type) string {
	parts := make([]string, 0, len(path)+1)
	for _, p := range path {
		parts = append(parts, typeName(p))
	}
	parts = append(parts, typeName(tail))
	return strings.Join(parts, " → ")
}

func typeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		return "*" + typeName(t.Elem())
	}
	if t.PkgPath() != "" {
		short := t.PkgPath()
		if i := strings.LastIndexByte(short, '/'); i >= 0 {
			short = short[i+1:]
		}
		return short + "." + t.Name()
	}
	return t.String()
}

// Scope implements container.Resolver.
func (c *Container) Scope() container.ScopedResolver {
	return &scopeImpl{root: c, instances: map[*binding]any{}}
}

// scopeImpl is a child scope: one instance per Scoped binding. Instances
// are keyed by binding (not type) so contextual and default bindings of
// the same type stay distinct.
type scopeImpl struct {
	root      *Container
	mu        sync.Mutex
	instances map[*binding]any
	order     []any // creation order, for reverse disposal
	closed    bool
}

func (s *scopeImpl) instanceOf(b *binding, path []reflect.Type) (any, error) {
	s.mu.Lock()
	if v, ok := s.instances[b]; ok {
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	v, err := s.root.construct(b, s, path)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("di: scope is closed; cannot resolve %s", typeName(b.key))
	}
	if prior, ok := s.instances[b]; ok { // lost a race; keep first
		return prior, nil
	}
	s.instances[b] = v
	s.order = append(s.order, v)
	return v, nil
}

func (s *scopeImpl) Resolve(key any) (any, error) {
	kt, err := keyType(key)
	if err != nil {
		return nil, err
	}
	return s.root.resolveType(kt, s, nil)
}

func (s *scopeImpl) Call(fn any) ([]any, error) { return call(s.root, s, fn) }

func (s *scopeImpl) Scope() container.ScopedResolver { return s.root.Scope() }

// Close disposes scoped instances in reverse creation order. Instances
// implementing io.Closer or Close(ctx) are closed; the first error is
// returned after all closers run.
func (s *scopeImpl) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	order := s.order
	s.order, s.instances = nil, nil
	s.mu.Unlock()

	var first error
	for i := len(order) - 1; i >= 0; i-- {
		switch inst := order[i].(type) {
		case interface{ Close(context.Context) error }:
			if err := inst.Close(ctx); err != nil && first == nil {
				first = err
			}
		case io.Closer:
			if err := inst.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}
