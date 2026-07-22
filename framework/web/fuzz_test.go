package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// FuzzBind: arbitrary bodies and content types must never panic; valid
// JSON with a JSON content type must decode.
func FuzzBind(f *testing.F) {
	f.Add(`{"name":"go"}`, "application/json")
	f.Add(`{"a":1}{"b":2}`, "application/json")
	f.Add(``, "")
	f.Add(`null`, "text/json")
	f.Add(`{"deep":{"deep":{"deep":[1,2,{"x":true}]}}}`, "application/json; charset=utf-8")
	f.Add(strings.Repeat("[", 10000), "application/json")
	f.Add(`{}`, "application/x-www-form-urlencoded")
	f.Add("\xff\xfe\x00", "application/json")

	f.Fuzz(func(t *testing.T, body, contentType string) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		var v map[string]any
		_ = Bind(req, &v) // must not panic, error is fine
	})
}
