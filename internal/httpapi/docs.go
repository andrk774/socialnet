package httpapi

import (
	"io/fs"
	"net/http"

	"socialnet/api"
)

// handleOpenAPI отдаёт спецификацию.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	b, err := api.Files.ReadFile("openapi.json")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// docsHandler — Swagger UI на /docs/. Своя CSP: скрипты только с нашего
// домена (без inline), стили Swagger UI требуют 'unsafe-inline'.
func docsHandler() http.Handler {
	sub, _ := fs.Sub(api.Files, "docs")
	files := http.StripPrefix("/docs/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}
