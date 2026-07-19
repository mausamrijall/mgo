// Package schema holds the ent schema for the blog example.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Post is the ent schema mirroring the app's Post shape.
type Post struct {
	ent.Schema
}

// Fields of the Post.
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("title"),
	}
}
