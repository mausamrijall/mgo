package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// CSRFCookie and CSRFHeader are the double-submit pair used by CSRF.
const (
	CSRFCookie = "mgo_csrf"
	CSRFHeader = "X-CSRF-Token"
)

// CSRF is stateless double-submit-cookie protection for session-backed
// routes. Safe methods (GET/HEAD/OPTIONS) ensure the token cookie exists;
// unsafe methods must echo the cookie's value in the X-CSRF-Token header
// or are rejected with 403. Token-authenticated APIs (Bearer) don't need
// this middleware — cookies aren't sent cross-origin by scripts anyway.
func CSRF() routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CSRFCookie)
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				if err != nil || cookie.Value == "" {
					var b [16]byte
					rand.Read(b[:])
					http.SetCookie(w, &http.Cookie{
						Name:     CSRFCookie,
						Value:    hex.EncodeToString(b[:]),
						Path:     "/",
						HttpOnly: false, // the client must read it to echo it
						SameSite: http.SameSiteLaxMode,
					})
				}
			default:
				token := r.Header.Get(CSRFHeader)
				if err != nil || cookie.Value == "" || token == "" ||
					subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) != 1 {
					http.Error(w, "csrf token mismatch", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
