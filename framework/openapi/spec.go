package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Spec assembles an OpenAPI document: baseline paths from the router's
// RouteLister capability, enriched per-operation with Describe.
type Spec struct {
	mu  sync.Mutex
	doc Document
}

// New starts a spec.
func New(info Info) *Spec {
	return &Spec{doc: Document{
		OpenAPI:    "3.1.0",
		Info:       info,
		Paths:      map[string]*PathItem{},
		Components: &Components{Schemas: map[string]*Schema{}},
	}}
}

// FromRouter builds the baseline spec from any router implementing the
// contracts/router.RouteLister capability (chi and stdmux adapters do):
// every registered route appears, path {params} become parameters —
// zero annotation required.
func FromRouter(r any, info Info) *Spec {
	s := New(info)
	if rl, ok := r.(routerc.RouteLister); ok {
		s.AddRoutes(rl.Routes()...)
	}
	return s
}

var paramRe = regexp.MustCompile(`\{([^}/]+)\}`)

// AddRoutes adds baseline entries for route metadata records. Routes
// without a method (mounted subtrees) and wildcard tails are skipped —
// enrich those by Describing concrete operations.
func (s *Spec) AddRoutes(routes ...routerc.Route) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rt := range routes {
		if rt.Method == "" || strings.Contains(rt.Pattern, "*") {
			continue
		}
		s.ensureOp(rt.Method, normalizePath(rt.Pattern))
	}
}

// normalizePath trims trailing slashes (except root) and the {$} stdmux
// end-anchor.
func normalizePath(p string) string {
	p = strings.TrimSuffix(p, "{$}")
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	if p == "" {
		p = "/"
	}
	return p
}

// ensureOp returns the operation for method+path, creating the baseline
// (path params, default 200) if absent. Caller holds the lock.
func (s *Spec) ensureOp(method, path string) *Operation {
	item := s.doc.Paths[path]
	if item == nil {
		item = &PathItem{}
		s.doc.Paths[path] = item
	}
	slot := item.op(strings.ToUpper(method))
	if slot == nil {
		return nil
	}
	if *slot == nil {
		op := &Operation{Responses: map[string]*Response{
			"200": {Description: "OK"},
		}}
		for _, m := range paramRe.FindAllStringSubmatch(path, -1) {
			op.Parameters = append(op.Parameters, Parameter{
				Name: m[1], In: "path", Required: true,
				Schema: &Schema{Types: []string{"string"}},
			})
		}
		*slot = op
	}
	return *slot
}

// Op is the enrichment for one operation — a pragmatic subset that
// compiles down to OpenAPI structures.
type Op struct {
	Summary     string
	Description string
	Tags        []string
	// Request is the JSON request-body schema (openapi.SchemaOf[T]()).
	Request *Schema
	// Responses maps status code → response. A nil schema means
	// body-less (204 and friends).
	Responses map[int]R
}

// R is one response's enrichment.
type R struct {
	Description string
	Schema      *Schema
}

// Describe enriches (or creates) the operation at "METHOD /path".
// Schemas produced by SchemaOf bring their named component schemas
// along; Describe hoists them into the document.
func (s *Spec) Describe(methodAndPath string, op Op) error {
	method, path, ok := strings.Cut(methodAndPath, " ")
	if !ok {
		return fmt.Errorf("openapi: Describe wants \"METHOD /path\", got %q", methodAndPath)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	o := s.ensureOp(strings.ToUpper(method), normalizePath(path))
	if o == nil {
		return fmt.Errorf("openapi: unsupported method %q", method)
	}
	o.Summary, o.Description, o.Tags = op.Summary, op.Description, op.Tags
	if op.Request != nil {
		s.hoist(op.Request)
		o.RequestBody = &RequestBody{Required: true, Content: map[string]MediaType{
			"application/json": {Schema: op.Request},
		}}
	}
	if len(op.Responses) > 0 {
		o.Responses = map[string]*Response{}
		for code, r := range op.Responses {
			resp := &Response{Description: r.Description}
			if resp.Description == "" {
				resp.Description = http.StatusText(code)
			}
			if r.Schema != nil {
				s.hoist(r.Schema)
				resp.Content = map[string]MediaType{"application/json": {Schema: r.Schema}}
			}
			o.Responses[fmt.Sprintf("%d", code)] = resp
		}
	}
	return nil
}

// hoist moves a schema's collected named definitions into components.
// Caller holds the lock.
func (s *Spec) hoist(sc *Schema) {
	for name, def := range sc.defs {
		s.doc.Components.Schemas[name] = def
	}
	sc.defs = nil
}

// Document returns a deep-enough copy for marshaling (paths sorted by
// the JSON encoder's map ordering — deterministic).
func (s *Spec) Document() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doc
}

// MarshalJSON emits the full document.
func (s *Spec) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Document())
}

// Handler serves the document as /openapi.json content.
func (s *Spec) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(s.Document())
	}
}

// SortedPaths returns the document's paths in order — for route:list
// style diagnostics and stable tests.
func (s *Spec) SortedPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.doc.Paths))
	for p := range s.doc.Paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
