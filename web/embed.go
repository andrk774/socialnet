// Package web — фронтенд (Preact + TypeScript), собранный в static/app.
// Исходники — web/ts/app, сборка: make web (tsc). Готовые файлы лежат в
// репозитории, чтобы Go-сборка не зависела от Node.
package web

import "embed"

//go:embed static
var Static embed.FS
