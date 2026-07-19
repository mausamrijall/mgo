// Package mgojwt adapts golang-jwt to MGO's auth contract: a Guard that
// authenticates Bearer tokens and an Issue helper that mints them. HS256
// only — asymmetric keys and OIDC verification are separate adapters.
package mgojwt

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	authc "github.com/mgo-framework/mgo/contracts/auth"
)

// Config for the guard and issuer.
type Config struct {
	// Secret signs and verifies tokens (HS256). Required, ≥32 bytes.
	Secret []byte
	// Issuer is set on minted tokens and enforced on verification when
	// non-empty.
	Issuer string
	// TTL is the minted token lifetime (default 15m).
	TTL time.Duration
}

// Identity is the authenticated JWT principal: the subject plus all
// claims, for handlers that need more than the id.
type Identity struct {
	Sub    string
	Claims jwt.MapClaims
}

// Subject implements contracts/auth.Identity.
func (i Identity) Subject() string { return i.Sub }

// Guard verifies Bearer tokens; it also mints them via Issue.
type Guard struct {
	cfg Config
}

var _ authc.Guard = (*Guard)(nil)

// New builds a Guard. Panics on a missing or short secret — that is a
// deployment error no request should ever reach.
func New(cfg Config) *Guard {
	if len(cfg.Secret) < 32 {
		panic("mgojwt: secret must be at least 32 bytes")
	}
	if cfg.TTL == 0 {
		cfg.TTL = 15 * time.Minute
	}
	return &Guard{cfg: cfg}
}

// Issue mints a signed token for sub with optional extra claims.
func (g *Guard) Issue(sub string, extra map[string]any) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": sub,
		"iat": now.Unix(),
		"exp": now.Add(g.cfg.TTL).Unix(),
	}
	if g.cfg.Issuer != "" {
		claims["iss"] = g.cfg.Issuer
	}
	for k, v := range extra {
		claims[k] = v
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(g.cfg.Secret)
}

// Authenticate implements contracts/auth.Guard for Authorization: Bearer.
func (g *Guard) Authenticate(r *http.Request) (authc.Identity, error) {
	header := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || raw == "" {
		return nil, authc.ErrUnauthenticated
	}

	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired()}
	if g.cfg.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(g.cfg.Issuer))
	}
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		return g.cfg.Secret, nil
	}, opts...)
	if err != nil || !token.Valid {
		return nil, authc.ErrUnauthenticated
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, authc.ErrUnauthenticated
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return nil, fmt.Errorf("mgojwt: token has no subject: %w", authc.ErrUnauthenticated)
	}
	return Identity{Sub: sub, Claims: claims}, nil
}
