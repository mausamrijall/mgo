package openapi

import (
	"fmt"
	"net/http"
)

// SwaggerUI serves an interactive API console for the spec at specURL
// (usually "/openapi.json"). The page is a minimal shell loading
// swagger-ui-dist from a CDN — nothing to embed, nothing to build, and
// air-gapped deployments can point users at the raw spec instead.
func SwaggerUI(specURL string) http.HandlerFunc {
	page := fmt.Sprintf(swaggerShell, specURL)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	}
}

const swaggerShell = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>API Documentation</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      SwaggerUIBundle({ url: %q, dom_id: "#swagger-ui" });
    };
  </script>
</body>
</html>
`
