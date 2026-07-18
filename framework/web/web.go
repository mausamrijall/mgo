// Package web holds MGO's request/response helpers as plain functions over
// http.ResponseWriter and *http.Request. There is intentionally no Ctx
// type: stdlib signatures stay first-class, these helpers are optional
// sugar, and deleting this package leaves ordinary net/http code.
package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// maxBindBytes caps request bodies read by Bind (1 MiB). Endpoints that
// need more should read r.Body themselves.
const maxBindBytes = 1 << 20

// JSON writes v as a JSON response with the given status.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// NoContent writes a body-less response with the given status.
func NoContent(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}

// Error writes {"error": message} with the given status.
func Error(w http.ResponseWriter, status int, message string) error {
	return JSON(w, status, map[string]string{"error": message})
}

// Bind decodes a JSON request body into v. It enforces a JSON (or absent)
// Content-Type, a 1 MiB body cap, and exactly one JSON value.
func Bind(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil || (mt != "application/json" && mt != "text/json") {
			return fmt.Errorf("web: unsupported content type %q", ct)
		}
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBindBytes))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("web: decode body: %w", err)
	}
	// Reject trailing garbage after the first JSON value.
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return errors.New("web: body must contain a single JSON value")
	}
	return nil
}
