package mgojwt_test

import (
	"net/http/httptest"
	"testing"
	"time"

	mgojwt "github.com/mgo-framework/mgo/adapters/auth-jwt"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

func TestIssueAndAuthenticate(t *testing.T) {
	g := mgojwt.New(mgojwt.Config{Secret: secret, Issuer: "mgo-test"})
	token, err := g.Issue("user-1", map[string]any{"role": "admin"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	id, err := g.Authenticate(req)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject() != "user-1" {
		t.Fatalf("subject = %q", id.Subject())
	}
	if jid, ok := id.(mgojwt.Identity); !ok || jid.Claims["role"] != "admin" {
		t.Fatalf("claims not carried: %+v", id)
	}
}

func TestRejectsMissingAndGarbageTokens(t *testing.T) {
	g := mgojwt.New(mgojwt.Config{Secret: secret})
	for _, header := range []string{"", "Bearer ", "Bearer garbage", "Basic abc"} {
		req := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if _, err := g.Authenticate(req); err == nil {
			t.Fatalf("header %q accepted", header)
		}
	}
}

func TestRejectsWrongSecretAndExpired(t *testing.T) {
	issuer := mgojwt.New(mgojwt.Config{Secret: secret, TTL: -time.Minute}) // already expired
	token, err := issuer.Issue("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := issuer.Authenticate(req); err == nil {
		t.Fatal("expired token accepted")
	}

	other := mgojwt.New(mgojwt.Config{Secret: []byte("ffffffffffffffffffffffffffffffff")})
	fresh, _ := mgojwt.New(mgojwt.Config{Secret: secret}).Issue("user-1", nil)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer "+fresh)
	if _, err := other.Authenticate(req2); err == nil {
		t.Fatal("token signed with different secret accepted")
	}
}

func TestRejectsWrongIssuer(t *testing.T) {
	minter := mgojwt.New(mgojwt.Config{Secret: secret, Issuer: "evil"})
	token, _ := minter.Issue("user-1", nil)
	strict := mgojwt.New(mgojwt.Config{Secret: secret, Issuer: "mgo"})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := strict.Authenticate(req); err == nil {
		t.Fatal("wrong issuer accepted")
	}
}

func TestShortSecretPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("short secret must panic")
		}
	}()
	mgojwt.New(mgojwt.Config{Secret: []byte("short")})
}
