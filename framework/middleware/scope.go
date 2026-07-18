package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mgo-framework/mgo/contracts/container"
	routerc "github.com/mgo-framework/mgo/contracts/router"
)

// Scope opens a container scope per request and closes it (disposing
// scoped instances in reverse creation order) when the handler returns.
// Handlers and services reach the scope with container.FromContext(ctx).
//
// Typically registered once, early in the chain:
//
//	router.Use(middleware.RequestID(), middleware.Scope(app.Container()))
func Scope(r container.Resolver) routerc.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			sc := r.Scope()
			defer func() {
				// The request context is likely done once the handler
				// returns; disposal gets a fresh, uncancelled context that
				// still carries the request's values.
				if err := sc.Close(context.WithoutCancel(req.Context())); err != nil {
					slog.ErrorContext(req.Context(), "request scope close failed",
						"error", err, "request_id", GetRequestID(req.Context()))
				}
			}()
			next.ServeHTTP(w, req.WithContext(container.NewContext(req.Context(), sc)))
		})
	}
}
