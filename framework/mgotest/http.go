package mgotest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ReqOption customizes a DSL request.
type ReqOption func(*http.Request)

// Header sets a request header.
func Header(key, value string) ReqOption {
	return func(r *http.Request) { r.Header.Set(key, value) }
}

// Bearer sets an Authorization: Bearer header.
func Bearer(token string) ReqOption {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

// Cookie attaches a cookie.
func Cookie(c *http.Cookie) ReqOption {
	return func(r *http.Request) { r.AddCookie(c) }
}

// Response wraps a recorded response with assertion helpers. The
// embedded *http.Response exposes everything else (headers, cookies).
type Response struct {
	*http.Response
	t    testing.TB
	body []byte
}

// Get performs a GET against the handler.
func Get(t testing.TB, h http.Handler, path string, opts ...ReqOption) *Response {
	t.Helper()
	return Do(t, h, httptest.NewRequest(http.MethodGet, path, nil), opts...)
}

// Post performs a POST with body JSON-encoded (nil = empty body).
func Post(t testing.TB, h http.Handler, path string, body any, opts ...ReqOption) *Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("mgotest: encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return Do(t, h, req, opts...)
}

// Delete performs a DELETE.
func Delete(t testing.TB, h http.Handler, path string, opts ...ReqOption) *Response {
	t.Helper()
	return Do(t, h, httptest.NewRequest(http.MethodDelete, path, nil), opts...)
}

// Do performs an arbitrary request against the handler.
func Do(t testing.TB, h http.Handler, req *http.Request, opts ...ReqOption) *Response {
	t.Helper()
	for _, opt := range opts {
		opt(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("mgotest: read body: %v", err)
	}
	resp.Body.Close()
	return &Response{Response: resp, t: t, body: body}
}

// RequireStatus fails the test unless the status matches.
func (r *Response) RequireStatus(code int) *Response {
	r.t.Helper()
	if r.StatusCode != code {
		r.t.Fatalf("mgotest: status = %d, want %d; body: %s", r.StatusCode, code, r.body)
	}
	return r
}

// JSON decodes the body into dst, failing the test on malformed JSON.
func (r *Response) JSON(dst any) *Response {
	r.t.Helper()
	if err := json.Unmarshal(r.body, dst); err != nil {
		r.t.Fatalf("mgotest: decode body: %v; body: %s", err, r.body)
	}
	return r
}

// Body returns the raw response body.
func (r *Response) Body() string { return string(r.body) }
