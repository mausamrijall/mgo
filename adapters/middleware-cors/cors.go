// Package mgocors is config-binding glue over github.com/rs/cors. It
// returns a stdlib-shaped middleware, imports nothing from MGO, and is
// fully deletable: replace New(cfg) with rs/cors directly at any time.
package mgocors

import (
	"net/http"

	"github.com/rs/cors"
)

// Config mirrors the rs/cors options MGO apps commonly bind from the
// "cors" config section. Zero value = rs/cors defaults (allow all origins,
// GET/POST/HEAD).
type Config struct {
	AllowedOrigins   []string `conf:"allowed_origins" json:"allowed_origins"`
	AllowedMethods   []string `conf:"allowed_methods" json:"allowed_methods"`
	AllowedHeaders   []string `conf:"allowed_headers" json:"allowed_headers"`
	ExposedHeaders   []string `conf:"exposed_headers" json:"exposed_headers"`
	AllowCredentials bool     `conf:"allow_credentials" json:"allow_credentials"`
	MaxAge           int      `conf:"max_age" json:"max_age"`
}

// New builds the CORS middleware from cfg.
func New(cfg Config) func(http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   cfg.AllowedMethods,
		AllowedHeaders:   cfg.AllowedHeaders,
		ExposedHeaders:   cfg.ExposedHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	}).Handler
}
