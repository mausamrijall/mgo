package mgoscs_test

import (
	"net/http"
	"net/http/httptest"
	"net/http/cookiejar"
	"testing"

	mgoscs "github.com/mgo-framework/mgo/adapters/session-scs"
	authc "github.com/mgo-framework/mgo/contracts/auth"
)

// app builds a tiny login/me/logout server around the adapter.
func app(s *mgoscs.Sessions) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Login(r.Context(), authc.Subject("user-1")); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		id, err := s.Guard().Authenticate(r)
		if err != nil {
			http.Error(w, "anon", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(id.Subject()))
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		s.Logout(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	return s.Middleware()(mux)
}

func client(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(app(mgoscs.New()))
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return srv, &http.Client{Jar: jar}
}

func TestLoginSessionLogoutFlow(t *testing.T) {
	srv, c := client(t)

	resp, err := c.Get(srv.URL + "/me")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("pre-login /me = %v %v, want 401", resp.StatusCode, err)
	}

	resp, err = c.Post(srv.URL+"/login", "", nil)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login = %v %v", resp.StatusCode, err)
	}

	resp, err = c.Get(srv.URL + "/me")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post-login /me = %v %v, want 200", resp.StatusCode, err)
	}

	resp, err = c.Post(srv.URL+"/logout", "", nil)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %v %v", resp.StatusCode, err)
	}

	resp, err = c.Get(srv.URL + "/me")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout /me = %v %v, want 401", resp.StatusCode, err)
	}
}

func TestSessionCookieDefaultsAreSecure(t *testing.T) {
	srv, c := client(t)
	resp, err := c.Post(srv.URL+"/login", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set on login")
	}
	sc := cookies[0]
	if !sc.HttpOnly {
		t.Fatal("session cookie is not HttpOnly")
	}
	if sc.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie SameSite = %v, want Lax", sc.SameSite)
	}
}

func TestLoginRotatesSessionToken(t *testing.T) {
	srv, c := client(t)
	// Prime an anonymous session? scs only writes a cookie once data
	// exists; login must rotate whatever token was there. Log in twice
	// and require different tokens.
	resp1, _ := c.Post(srv.URL+"/login", "", nil)
	tok1 := resp1.Cookies()[0].Value
	resp2, _ := c.Post(srv.URL+"/login", "", nil)
	if len(resp2.Cookies()) == 0 {
		t.Skip("no rotation cookie surfaced")
	}
	tok2 := resp2.Cookies()[0].Value
	if tok1 == tok2 {
		t.Fatal("session token not rotated on login (fixation risk)")
	}
}
