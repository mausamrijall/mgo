package openapi

import (
	"reflect"
	"strings"
	"time"
)

// SchemaOf derives a JSON Schema from T by reflection: json tags name
// properties ("-" skips), fields without omitempty are required,
// pointers become nullable (["T","null"]), time.Time is date-time,
// nested named structs land in components as $ref (cycle-safe).
//
// The returned schema may contain $ref entries; Spec.Describe resolves
// them into the document's components automatically.
func SchemaOf[T any]() *Schema {
	g := &schemaGen{defs: map[string]*Schema{}}
	s := g.schema(reflect.TypeFor[T](), map[reflect.Type]bool{})
	s.defs = g.defs
	return s
}

type schemaGen struct {
	defs map[string]*Schema
}

func (g *schemaGen) schema(t reflect.Type, visiting map[reflect.Type]bool) *Schema {
	switch t.Kind() {
	case reflect.Pointer:
		inner := g.schema(t.Elem(), visiting)
		if inner.Ref != "" {
			// $ref cannot carry siblings pre-3.1 tools reliably; wrap not
			// needed — nullability of referenced objects stays implicit.
			return inner
		}
		inner.Types = append(inner.Types, "null")
		return inner
	case reflect.Bool:
		return &Schema{Types: []string{"boolean"}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Types: []string{"integer"}}
	case reflect.Float32, reflect.Float64:
		return &Schema{Types: []string{"number"}}
	case reflect.String:
		return &Schema{Types: []string{"string"}}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Types: []string{"string"}, Format: "byte"}
		}
		return &Schema{Types: []string{"array"}, Items: g.schema(t.Elem(), visiting)}
	case reflect.Map:
		return &Schema{Types: []string{"object"}, AdditionalProperties: g.schema(t.Elem(), visiting)}
	case reflect.Struct:
		if t == reflect.TypeOf(time.Time{}) {
			return &Schema{Types: []string{"string"}, Format: "date-time"}
		}
		return g.structSchema(t, visiting)
	case reflect.Interface:
		return &Schema{} // any
	default:
		return &Schema{}
	}
}

func (g *schemaGen) structSchema(t reflect.Type, visiting map[reflect.Type]bool) *Schema {
	named := t.Name() != ""
	if named {
		if visiting[t] {
			return &Schema{Ref: "#/components/schemas/" + t.Name()}
		}
		if _, done := g.defs[t.Name()]; done {
			return &Schema{Ref: "#/components/schemas/" + t.Name()}
		}
		visiting[t] = true
		defer delete(visiting, t)
	}

	obj := &Schema{Types: []string{"object"}, Properties: map[string]*Schema{}}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			// Embedded struct: fold its properties in (json flattening).
			emb := g.schema(f.Type, visiting)
			if emb.Ref == "" {
				for k, v := range emb.Properties {
					obj.Properties[k] = v
				}
				obj.Required = append(obj.Required, emb.Required...)
				continue
			}
		}
		name, omitempty, skip := jsonName(f)
		if skip {
			continue
		}
		fs := g.schema(f.Type, visiting)
		if d := f.Tag.Get("doc"); d != "" && fs.Ref == "" {
			fs.Description = d
		}
		obj.Properties[name] = fs
		if !omitempty {
			obj.Required = append(obj.Required, name)
		}
	}

	if named {
		g.defs[t.Name()] = obj
		return &Schema{Ref: "#/components/schemas/" + t.Name()}
	}
	return obj
}

func jsonName(f reflect.StructField) (name string, omitempty, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	name = f.Name
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	for _, p := range parts[1:] {
		if p == "omitempty" || p == "omitzero" {
			omitempty = true
		}
	}
	return name, omitempty, false
}
