// Package di implements the MGO service container (contracts/container).
//
// Design (docs/architecture/02 §6): reflection happens once at registration —
// constructor signatures are inspected and cached as typed plans. Validate()
// walks the graph at boot, reporting missing bindings, cycles, and lifetime
// violations with full resolution chains. Runtime resolution executes the
// cached plan: map lookups and function calls only.
package di

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/mgo-framework/mgo/contracts/container"
)

var errType = reflect.TypeOf((*error)(nil)).Elem()

// binding is a registered constructor with its pre-computed plan.
type binding struct {
	key      reflect.Type
	lifetime container.Lifetime
	ctor     reflect.Value
	params   []reflect.Type // constructor parameter types (the plan)
	hasErr   bool           // constructor returns (T, error)

	once     sync.Once // singleton memoization
	instance reflect.Value
	buildErr error
}

// Container is the root implementation of contracts/container.Container.
type Container struct {
	mu       sync.RWMutex
	bindings map[reflect.Type]*binding
	sealed   bool // set by Validate; further registration is an error
}

// New creates an empty root container.
func New() *Container {
	return &Container{bindings: map[reflect.Type]*binding{}}
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
	return c.bind(kt, constructor, lifetime)
}

func (c *Container) bind(kt reflect.Type, constructor any, lifetime container.Lifetime) error {
	cv := reflect.ValueOf(constructor)
	ct := cv.Type()
	if ct == nil || ct.Kind() != reflect.Func {
		return fmt.Errorf("di: constructor for %s must be a function, got %T", typeName(kt), constructor)
	}
	if ct.NumOut() == 0 || ct.NumOut() > 2 {
		return fmt.Errorf("di: constructor for %s must return (T) or (T, error)", typeName(kt))
	}
	hasErr := ct.NumOut() == 2
	if hasErr && ct.Out(1) != errType {
		return fmt.Errorf("di: constructor for %s second return must be error, got %s", typeName(kt), ct.Out(1))
	}
	if out := ct.Out(0); !out.AssignableTo(kt) {
		return fmt.Errorf("di: constructor for %s returns %s which is not assignable to it", typeName(kt), typeName(out))
	}
	params := make([]reflect.Type, ct.NumIn())
	for i := range params {
		params[i] = ct.In(i)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return fmt.Errorf("di: container is sealed (Validate ran); cannot bind %s", typeName(kt))
	}
	c.bindings[kt] = &binding{key: kt, lifetime: lifetime, ctor: cv, params: params, hasErr: hasErr}
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
		return fmt.Errorf("di: instance value %T is not assignable to %s", value, typeName(kt))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return fmt.Errorf("di: container is sealed; cannot register instance %s", typeName(kt))
	}
	b := &binding{key: kt, lifetime: container.Singleton, instance: vv}
	b.once.Do(func() {}) // already built
	c.bindings[kt] = b
	return nil
}

// lookup returns the binding for t, trying auto-resolution for unregistered
// concrete struct/pointer-to-struct types (doc 02 §6.4).
func (c *Container) lookup(t reflect.Type) (*binding, bool) {
	c.mu.RLock()
	b, ok := c.bindings[t]
	c.mu.RUnlock()
	return b, ok
}

// Resolve implements container.Resolver on the root: Scoped bindings error.
func (c *Container) Resolve(key any) (any, error) {
	kt, err := keyType(key)
	if err != nil {
		return nil, err
	}
	v, err := c.resolveType(kt, nil, nil)
	if err != nil {
		return nil, err
	}
	return v.Interface(), nil
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

// resolveType resolves t. scope is nil for root resolution; path carries the
// current chain for diagnostics and cycle detection.
func (c *Container) resolveType(t reflect.Type, scope *scopeImpl, path []reflect.Type) (reflect.Value, error) {
	for _, p := range path {
		if p == t {
			return reflect.Value{}, fmt.Errorf("di: dependency cycle: %s", chain(path, t))
		}
	}

	b, ok := c.lookup(t)
	if !ok {
		// Auto-resolution: unregistered concrete types with resolvable deps.
		if ab, err := c.autoBinding(t); err == nil {
			b = ab
		} else {
			hint := ""
			if t.Kind() == reflect.Interface {
				hint = " (interfaces require an explicit binding)"
			}
			return reflect.Value{}, fmt.Errorf("di: no binding for %s%s; chain: %s", typeName(t), hint, chain(path, t))
		}
	}

	switch b.lifetime {
	case container.Singleton:
		b.once.Do(func() {
			b.instance, b.buildErr = c.construct(b, nil, append(path, t))
		})
		return b.instance, b.buildErr
	case container.Scoped:
		if scope == nil {
			return reflect.Value{}, fmt.Errorf("di: %s is Scoped and cannot be resolved from the root container; chain: %s", typeName(t), chain(path, t))
		}
		return scope.instanceOf(b, append(path, t))
	default: // Transient
		return c.construct(b, scope, append(path, t))
	}
}

// autoBinding builds a transient binding for an unregistered concrete type
// whose zero-arg-or-resolvable constructor is synthesized from its identity.
// Only *struct and struct kinds participate.
func (c *Container) autoBinding(t reflect.Type) (*binding, error) {
	base := t
	if base.Kind() == reflect.Ptr {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return nil, fmt.Errorf("di: cannot auto-resolve %s", typeName(t))
	}
	// Synthesized constructor: allocate zero value. Field injection is
	// deliberately not supported — constructor injection only (doc 02 §6.4).
	ctor := reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{t}, false),
		func([]reflect.Value) []reflect.Value {
			if t.Kind() == reflect.Ptr {
				return []reflect.Value{reflect.New(t.Elem())}
			}
			return []reflect.Value{reflect.New(t).Elem()}
		})
	return &binding{key: t, lifetime: container.Transient, ctor: ctor, params: nil}, nil
}

// construct invokes a binding's constructor, resolving parameters.
func (c *Container) construct(b *binding, scope *scopeImpl, path []reflect.Type) (reflect.Value, error) {
	args := make([]reflect.Value, len(b.params))
	for i, pt := range b.params {
		v, err := c.resolveType(pt, scope, path)
		if err != nil {
			return reflect.Value{}, err
		}
		args[i] = v
	}
	out := b.ctor.Call(args)
	if b.hasErr && !out[1].IsNil() {
		return reflect.Value{}, fmt.Errorf("di: constructor for %s failed: %w", typeName(b.key), out[1].Interface().(error))
	}
	return out[0], nil
}

// Call implements container.Resolver.Call on the root container.
func (c *Container) Call(fn any) ([]any, error) {
	return call(c, nil, fn)
}

func call(c *Container, scope *scopeImpl, fn any) ([]any, error) {
	fv := reflect.ValueOf(fn)
	ft := fv.Type()
	if ft == nil || ft.Kind() != reflect.Func {
		return nil, fmt.Errorf("di: Call target must be a function, got %T", fn)
	}
	args := make([]reflect.Value, ft.NumIn())
	for i := range args {
		v, err := c.resolveType(ft.In(i), scope, nil)
		if err != nil {
			return nil, err
		}
		args[i] = v
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

// Validate implements container.Container: full-graph boot-time check.
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
		if err := c.check(k, nil, map[reflect.Type]bool{}); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("di: graph validation failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// check walks dependencies of k without constructing anything.
func (c *Container) check(k reflect.Type, path []reflect.Type, visiting map[reflect.Type]bool) error {
	if visiting[k] {
		return fmt.Errorf("dependency cycle: %s", chain(path, k))
	}
	b, ok := c.lookup(k)
	if !ok {
		if _, err := c.autoBinding(k); err != nil {
			hint := ""
			if k.Kind() == reflect.Interface {
				hint = " (interfaces require an explicit binding)"
			}
			return fmt.Errorf("no binding for %s%s; chain: %s", typeName(k), hint, chain(path, k))
		}
		return nil
	}
	// Lifetime rule: a Singleton must not depend on a Scoped binding.
	if len(path) > 0 {
		if root, _ := c.lookup(path[0]); root != nil && root.lifetime == container.Singleton && b.lifetime == container.Scoped {
			return fmt.Errorf("lifetime violation: singleton %s depends on scoped %s; chain: %s", typeName(path[0]), typeName(k), chain(path, k))
		}
	}
	visiting[k] = true
	defer delete(visiting, k)
	for _, p := range b.params {
		if err := c.check(p, append(path, k), visiting); err != nil {
			return err
		}
	}
	return nil
}

// Scope implements container.Resolver.
func (c *Container) Scope() container.ScopedResolver {
	return &scopeImpl{root: c, instances: map[reflect.Type]reflect.Value{}}
}

// scopeImpl is a child scope: per-scope instances for Scoped bindings.
type scopeImpl struct {
	root      *Container
	mu        sync.Mutex
	instances map[reflect.Type]reflect.Value
	order     []reflect.Value // creation order, for reverse disposal
	closed    bool
}

func (s *scopeImpl) instanceOf(b *binding, path []reflect.Type) (reflect.Value, error) {
	s.mu.Lock()
	if v, ok := s.instances[b.key]; ok {
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	v, err := s.root.construct(b, s, path)
	if err != nil {
		return reflect.Value{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return reflect.Value{}, fmt.Errorf("di: scope is closed; cannot resolve %s", typeName(b.key))
	}
	if prior, ok := s.instances[b.key]; ok { // lost a race; keep first
		return prior, nil
	}
	s.instances[b.key] = v
	s.order = append(s.order, v)
	return v, nil
}

func (s *scopeImpl) Resolve(key any) (any, error) {
	kt, err := keyType(key)
	if err != nil {
		return nil, err
	}
	v, err := s.root.resolveType(kt, s, nil)
	if err != nil {
		return nil, err
	}
	return v.Interface(), nil
}

func (s *scopeImpl) Call(fn any) ([]any, error) { return call(s.root, s, fn) }

func (s *scopeImpl) Scope() container.ScopedResolver { return s.root.Scope() }

// Close disposes scoped instances in reverse creation order. Instances
// implementing io.Closer or Closable(ctx) are closed; the first error is
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
		switch inst := order[i].Interface().(type) {
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
