// Package openapi generates OpenAPI 3.1 documents from MGO routers —
// first-party, stdlib-only, and additive: the router keeps its native
// API (r.Post("/users", h) is still chi), the baseline spec comes free
// from the contracts/router.RouteLister capability, and schemas are
// added per-operation with Describe. The document is the gateway
// artifact: Swagger UI ships here; SDKs and Postman collections are one
// openapi-generator invocation away from /openapi.json.
package openapi

import "encoding/json"

// Document is the OpenAPI 3.1 root object (the subset MGO emits).
type Document struct {
	OpenAPI    string               `json:"openapi"`
	Info       Info                 `json:"info"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
}

// Info describes the API.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// PathItem holds one path's operations keyed by lowercase HTTP method.
type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Head   *Operation `json:"head,omitempty"`
}

// op returns the operation slot for method, creating it on demand.
func (p *PathItem) op(method string) **Operation {
	switch method {
	case "GET":
		return &p.Get
	case "POST":
		return &p.Post
	case "PUT":
		return &p.Put
	case "PATCH":
		return &p.Patch
	case "DELETE":
		return &p.Delete
	case "HEAD":
		return &p.Head
	}
	return nil
}

// Operation is one method+path entry.
type Operation struct {
	Summary     string               `json:"summary,omitempty"`
	Description string               `json:"description,omitempty"`
	OperationID string               `json:"operationId,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Parameters  []Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses"`
}

// Parameter is a path/query/header parameter.
type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"` // path | query | header
	Required bool    `json:"required,omitempty"`
	Schema   *Schema `json:"schema,omitempty"`
}

// RequestBody wraps a JSON request schema.
type RequestBody struct {
	Required bool                 `json:"required,omitempty"`
	Content  map[string]MediaType `json:"content"`
}

// Response is one status-code entry.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType binds a schema to a content type.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Components holds shared schemas referenced with $ref.
type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty"`
}

// Schema is a JSON Schema (OpenAPI 3.1 = full JSON Schema dialect).
// Type is string or ["type","null"] — Types marshals correctly for both.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	Types                []string           `json:"-"`
	Format               string             `json:"format,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`

	// defs carries named schemas collected by SchemaOf; Spec hoists them
	// into components. Unexported: never serialized.
	defs map[string]*Schema
}

// MarshalJSON emits "type" as a string for one type and an array for
// several (the 3.1 nullable idiom ["string","null"]).
func (s *Schema) MarshalJSON() ([]byte, error) {
	type alias Schema // no method set: avoids recursion
	raw, err := json.Marshal((*alias)(s))
	if err != nil || len(s.Types) == 0 {
		return raw, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if len(s.Types) == 1 {
		m["type"], _ = json.Marshal(s.Types[0])
	} else {
		m["type"], _ = json.Marshal(s.Types)
	}
	return json.Marshal(m)
}
