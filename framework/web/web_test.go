package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := JSON(rec, 201, map[string]int{"n": 7}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 201 {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"n":7}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestBind(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"go"}`))
	req.Header.Set("Content-Type", "application/json")
	var v struct{ Name string }
	if err := Bind(req, &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "go" {
		t.Fatalf("Name = %q", v.Name)
	}
}

func TestBindRejectsWrongContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/xml")
	if err := Bind(req, &struct{}{}); err == nil {
		t.Fatal("expected content-type error")
	}
}

func TestBindRejectsTrailingGarbage(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"a":1}{"b":2}`))
	req.Header.Set("Content-Type", "application/json")
	if err := Bind(req, &struct{ A int }{}); err == nil {
		t.Fatal("expected single-value error")
	}
}

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Error(rec, 404, "nope"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"nope"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
