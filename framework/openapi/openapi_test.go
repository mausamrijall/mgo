package openapi_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	routerc "github.com/mgo-framework/mgo/contracts/router"
	"github.com/mgo-framework/mgo/framework/openapi"
)

// ---- schema reflection ----

type Author struct {
	Name string `json:"name"`
}

type Post struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title" doc:"Post headline"`
	Draft     bool      `json:"draft,omitempty"`
	Author    Author    `json:"author"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Secret    string    `json:"-"`
	Note      *string   `json:"note,omitempty"`
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSchemaOfStruct(t *testing.T) {
	spec := openapi.New(openapi.Info{Title: "t", Version: "1"})
	if err := spec.Describe("POST /posts", openapi.Op{
		Request:   openapi.SchemaOf[Post](),
		Responses: map[int]openapi.R{201: {Schema: openapi.SchemaOf[Post]()}},
	}); err != nil {
		t.Fatal(err)
	}
	out := marshal(t, spec)

	for _, want := range []string{
		`"$ref":"#/components/schemas/Post"`,   // named structs are referenced
		`"$ref":"#/components/schemas/Author"`, // nested named struct too
		`"created_at":{"format":"date-time","type":"string"}`,
		`"Post headline"`,                   // doc tag → description
		`"note":{"type":["string","null"]}`, // pointer → nullable
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("document missing %s\nin: %s", want, out)
		}
	}
	if strings.Contains(out, "Secret") || strings.Contains(out, `"secret"`) {
		t.Fatal("json:\"-\" field leaked into the schema")
	}

	// Required = fields without omitempty (id, title, author, created_at).
	var doc map[string]any
	json.Unmarshal([]byte(out), &doc)
	post := doc["components"].(map[string]any)["schemas"].(map[string]any)["Post"].(map[string]any)
	req := post["required"].([]any)
	if len(req) != 4 {
		t.Fatalf("required = %v, want the 4 non-omitempty fields", req)
	}
}

func TestSchemaOfPrimitivesAndContainers(t *testing.T) {
	if got := marshal(t, openapi.SchemaOf[[]int]()); got != `{"items":{"type":"integer"},"type":"array"}` {
		t.Fatalf("slice schema = %s", got)
	}
	if got := marshal(t, openapi.SchemaOf[map[string]float64]()); got != `{"additionalProperties":{"type":"number"},"type":"object"}` {
		t.Fatalf("map schema = %s", got)
	}
}

type Node struct {
	Value    string  `json:"value"`
	Children []*Node `json:"children,omitempty"`
}

func TestSchemaOfCyclicType(t *testing.T) {
	// Must terminate and self-reference.
	s := openapi.SchemaOf[Node]()
	spec := openapi.New(openapi.Info{Title: "t", Version: "1"})
	spec.Describe("POST /nodes", openapi.Op{Request: s})
	out := marshal(t, spec)
	if !strings.Contains(out, `"children":{"items":{"$ref":"#/components/schemas/Node"}`) {
		t.Fatalf("cycle not $ref'd: %s", out)
	}
}

// ---- baseline from RouteLister ----

type fakeRouter struct{ routes []routerc.Route }

func (f fakeRouter) Routes() []routerc.Route { return f.routes }

func TestFromRouterBaseline(t *testing.T) {
	spec := openapi.FromRouter(fakeRouter{routes: []routerc.Route{
		{Method: "GET", Pattern: "/posts"},
		{Method: "POST", Pattern: "/posts"},
		{Method: "GET", Pattern: "/posts/{id}"},
		{Method: "GET", Pattern: "/{$}"},    // stdmux root anchor
		{Method: "", Pattern: "/mounted/"},  // mounts skipped
		{Method: "GET", Pattern: "/wild/*"}, // wildcards skipped
	}}, openapi.Info{Title: "Blog", Version: "1.0.0"})

	if got := spec.SortedPaths(); strings.Join(got, ",") != "/,/posts,/posts/{id}" {
		t.Fatalf("paths = %v", got)
	}
	out := marshal(t, spec)
	if !strings.Contains(out, `"name":"id","in":"path","required":true`) {
		t.Fatalf("path param not derived: %s", out)
	}
	if !strings.Contains(out, `"openapi":"3.1.0"`) {
		t.Fatal("missing version")
	}
}

func TestDescribeMergesOverBaseline(t *testing.T) {
	spec := openapi.FromRouter(fakeRouter{routes: []routerc.Route{
		{Method: "GET", Pattern: "/posts/{id}"},
	}}, openapi.Info{Title: "t", Version: "1"})
	if err := spec.Describe("GET /posts/{id}", openapi.Op{
		Summary:   "Fetch one post",
		Responses: map[int]openapi.R{200: {Schema: openapi.SchemaOf[Post]()}, 404: {}},
	}); err != nil {
		t.Fatal(err)
	}
	out := marshal(t, spec)
	if !strings.Contains(out, `"summary":"Fetch one post"`) {
		t.Fatal("summary not merged")
	}
	if !strings.Contains(out, `"404":{"description":"Not Found"}`) {
		t.Fatalf("status text default missing: %s", out)
	}
	// Baseline path param survives enrichment.
	if !strings.Contains(out, `"name":"id"`) {
		t.Fatal("path param lost on Describe")
	}
}

// ---- serving ----

func TestHandlersServeJSONAndUI(t *testing.T) {
	spec := openapi.New(openapi.Info{Title: "t", Version: "1"})

	rec := httptest.NewRecorder()
	spec.Handler()(rec, httptest.NewRequest("GET", "/openapi.json", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("spec content type = %q", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("served spec is not JSON: %v", err)
	}

	rec = httptest.NewRecorder()
	openapi.SwaggerUI("/openapi.json")(rec, httptest.NewRequest("GET", "/docs", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") || !strings.Contains(body, `"/openapi.json"`) {
		t.Fatal("swagger shell malformed")
	}
}
