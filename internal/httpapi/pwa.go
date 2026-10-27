package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// buildServiceWorker подставляет в web/static/sw.js список файлов оболочки
// и версию (хеш их ETag): поменялся любой файл — поменялся и воркер.
func buildServiceWorker(static map[string]staticFile) []byte {
	tpl, ok := static["/static/sw.js"]
	if !ok {
		return nil
	}
	var assets []string
	for name := range static {
		if name == "/static/sw.js" || name == "/static/index.html" || strings.HasSuffix(name, "/LICENSE") ||
			strings.HasSuffix(name, "/VERSION") {
			continue
		}
		assets = append(assets, name)
	}
	sort.Strings(assets)
	h := sha256.New()
	for _, a := range assets {
		h.Write([]byte(a + static[a].etag))
	}
	h.Write([]byte(static["/static/index.html"].etag))
	list, _ := json.Marshal(assets)
	out := bytes.Replace(tpl.data, []byte("__ASSETS__"), list, 1)
	return bytes.Replace(out, []byte("__VERSION__"), []byte(hex.EncodeToString(h.Sum(nil)[:6])), 1)
}

// handleServiceWorker: GET /sw.js — с корня сайта, чтобы управлять всеми страницами.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	if s.sw == nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache") // браузер проверяет обновление при каждом заходе
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", s.pageCSP())
	_, _ = w.Write(s.sw)
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	f, ok := s.static["/static/manifest.webmanifest"]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(f.data)
}
