package main

// The Phase 6 exit: the ENTIRE feature suite runs on both router
// adapters, plus a security-defaults audit.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mgo-framework/mgo/framework/middleware"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

type client struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newClient(t *testing.T, router string) *client {
	t.Helper()
	a, err := newApp(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.handler(router))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, srv: srv, c: &http.Client{Jar: jar}}
}

// csrf fetches (and returns) the CSRF token by hitting a safe endpoint.
func (cl *client) csrf() string {
	resp, err := cl.c.Get(cl.srv.URL + "/me") // 401, but sets the cookie
	if err != nil {
		cl.t.Fatal(err)
	}
	resp.Body.Close()
	u, _ := url.Parse(cl.srv.URL)
	for _, c := range cl.c.Jar.Cookies(u) {
		if c.Name == middleware.CSRFCookie {
			return c.Value
		}
	}
	cl.t.Fatal("no csrf cookie issued")
	return ""
}

func (cl *client) post(path, csrf, bearer string, body any) *http.Response {
	cl.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest("POST", cl.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(middleware.CSRFHeader, csrf)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := cl.c.Do(req)
	if err != nil {
		cl.t.Fatal(err)
	}
	cl.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (cl *client) get(path, bearer string) *http.Response {
	cl.t.Helper()
	req, _ := http.NewRequest("GET", cl.srv.URL+path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := cl.c.Do(req)
	if err != nil {
		cl.t.Fatal(err)
	}
	cl.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (cl *client) token(email, password string) string {
	cl.t.Helper()
	resp := cl.post("/token", "", "", map[string]string{"email": email, "password": password})
	if resp.StatusCode != 200 {
		cl.t.Fatalf("token request = %d", resp.StatusCode)
	}
	var out struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&out)
	return out.Token
}

// eachRouter runs the test body against chi and stdmux.
func eachRouter(t *testing.T, fn func(t *testing.T, cl *client)) {
	for _, router := range []string{"chi", "stdmux"} {
		t.Run(router, func(t *testing.T) { fn(t, newClient(t, router)) })
	}
}

func TestSessionLoginFlow(t *testing.T) {
	eachRouter(t, func(t *testing.T, cl *client) {
		csrf := cl.csrf()

		if resp := cl.post("/login", csrf, "", map[string]string{"email": "user@example.com", "password": "wrong"}); resp.StatusCode != 401 {
			t.Fatalf("wrong password = %d, want 401", resp.StatusCode)
		}
		if resp := cl.post("/login", csrf, "", map[string]string{"email": "user@example.com", "password": "user123"}); resp.StatusCode != 204 {
			t.Fatalf("login = %d, want 204", resp.StatusCode)
		}
		resp := cl.get("/me", "")
		if resp.StatusCode != 200 {
			t.Fatalf("/me = %d, want 200", resp.StatusCode)
		}
		var me struct{ Subject string }
		json.NewDecoder(resp.Body).Decode(&me)
		if me.Subject != "user@example.com" {
			t.Fatalf("subject = %q", me.Subject)
		}
		if resp := cl.post("/logout", csrf, "", nil); resp.StatusCode != 204 {
			t.Fatalf("logout = %d, want 204", resp.StatusCode)
		}
		if resp := cl.get("/me", ""); resp.StatusCode != 401 {
			t.Fatalf("post-logout /me = %d, want 401", resp.StatusCode)
		}
	})
}

func TestJWTAPIFlow(t *testing.T) {
	eachRouter(t, func(t *testing.T, cl *client) {
		if resp := cl.get("/api/data", ""); resp.StatusCode != 401 {
			t.Fatalf("anonymous api = %d, want 401", resp.StatusCode)
		}
		if resp := cl.get("/api/data", "garbage.token.here"); resp.StatusCode != 401 {
			t.Fatalf("garbage token = %d, want 401", resp.StatusCode)
		}
		token := cl.token("user@example.com", "user123")
		resp := cl.get("/api/data", token)
		if resp.StatusCode != 200 {
			t.Fatalf("bearer api = %d, want 200", resp.StatusCode)
		}
		var out struct{ Sub string }
		json.NewDecoder(resp.Body).Decode(&out)
		if out.Sub != "user@example.com" {
			t.Fatalf("sub = %q", out.Sub)
		}
	})
}

func TestPolicyProtectedResource(t *testing.T) {
	eachRouter(t, func(t *testing.T, cl *client) {
		if resp := cl.get("/admin", ""); resp.StatusCode != 401 {
			t.Fatalf("anonymous admin = %d, want 401", resp.StatusCode)
		}
		userTok := cl.token("user@example.com", "user123")
		if resp := cl.get("/admin", userTok); resp.StatusCode != 403 {
			t.Fatalf("member admin = %d, want 403", resp.StatusCode)
		}
		adminTok := cl.token("admin@example.com", "admin123")
		if resp := cl.get("/admin", adminTok); resp.StatusCode != 200 {
			t.Fatalf("admin admin = %d, want 200", resp.StatusCode)
		}
	})
}

// TestSecurityDefaultsAudit is the exit's security audit: headers,
// cookie flags, and CSRF enforcement.
func TestSecurityDefaultsAudit(t *testing.T) {
	eachRouter(t, func(t *testing.T, cl *client) {
		// Hardening headers on every response.
		resp := cl.get("/me", "")
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff")
		}
		if resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Fatal("missing frame deny")
		}

		// Login without a CSRF token is rejected.
		if resp := cl.post("/login", "", "", map[string]string{"email": "user@example.com", "password": "user123"}); resp.StatusCode != 403 {
			t.Fatalf("csrf-less login = %d, want 403", resp.StatusCode)
		}

		// Session cookie: HttpOnly + SameSite.
		csrf := cl.csrf()
		loginResp := cl.post("/login", csrf, "", map[string]string{"email": "user@example.com", "password": "user123"})
		found := false
		for _, c := range loginResp.Cookies() {
			if strings.Contains(c.Name, "session") {
				found = true
				if !c.HttpOnly {
					t.Fatal("session cookie not HttpOnly")
				}
				if c.SameSite != http.SameSiteLaxMode {
					t.Fatalf("session cookie SameSite = %v, want Lax", c.SameSite)
				}
			}
		}
		if !found {
			t.Fatal("no session cookie on login")
		}

		// Passwords at rest are argon2id PHC strings, never plaintext.
		a, _ := newApp(testSecret)
		for _, u := range a.users {
			if !strings.HasPrefix(u.hash, "$argon2id$v=19$") {
				t.Fatalf("password stored as %q — not argon2id", u.hash[:20])
			}
		}
	})
}
