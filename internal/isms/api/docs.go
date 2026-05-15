package api

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed openapi.yaml
var openAPISpec []byte

func (s *Server) registerDocs() {
	// Serve OpenAPI spec
	s.echo.GET("/api/openapi.yaml", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml", openAPISpec)
	})

	// Serve Scalar API reference UI at /docs
	s.echo.GET("/docs", func(c echo.Context) error {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>isms.sh API Documentation</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
    <script id="api-reference" data-url="/api/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
		return c.HTML(http.StatusOK, html)
	})
}
