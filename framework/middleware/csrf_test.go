package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mgo-framework/mgo/framework/middleware"
)

func csrfApp() http.Handler {
	return middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), middleware.CSRF())
}

func TestCSRFIssuesTokenOnSafeMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	csrfApp().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != middleware.CSRFCookie || cookies[0].Value == "" {
		t.Fatalf("expected csrf cookie, got %v", cookies)
	}
}

func TestCSRFRejectsUnsafeWithoutToken(t *testing.T) {
	rec := httptest.NewRecorder()
	csrfApp().ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCSRFAcceptsMatchingPair(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookie, Value: "tok123"})
	req.Header.Set(middleware.CSRFHeader, "tok123")
	rec := httptest.NewRecorder()
	csrfApp().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestCSRFRejectsMismatchedPair(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookie, Value: "tok123"})
	req.Header.Set(middleware.CSRFHeader, "other")
	rec := httptest.NewRecorder()
	csrfApp().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
