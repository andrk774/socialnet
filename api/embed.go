// Package api содержит спецификацию OpenAPI и страницу документации.
package api

import "embed"

// Files: openapi.json и docs/ (Swagger UI).
//
//go:embed openapi.json docs
var Files embed.FS
