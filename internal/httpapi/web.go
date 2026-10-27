package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"socialnet/web"
)

type staticFile struct {
	data  []byte
	etag  string
	ctype string
}

// loadStatic читает встроенные файлы и считает ETag заранее: у embed.FS нет
// времени изменения, а без валидатора браузер скачивал бы всё заново.
func loadStatic() map[string]staticFile {
	out := map[string]staticFile{}
	_ = fs.WalkDir(web.Static, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(web.Static, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		if strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".d.ts") {
			return nil // типы TypeScript браузеру не нужны
		}
		out["/"+p] = staticFile{data: data, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, ctype: ct}
		return nil
	})
	return out
}

// pageCSP — для HTML и скриптов фронтенда. Никакого inline-JS и eval:
// внедрённый через XSS скрипт не выполнится. Медиа и картинки — свои и CDN.
func (s *Server) pageCSP() string {
	media := "'self' blob:"
	if s.cfg.MediaCDNURL != "" {
		media += " " + s.cfg.MediaCDNURL
	}
	script, style, connect, frame := "'self'", "'self'", "'self'", ""
	if s.captchaEnabled() { // виджет SmartCaptcha: скрипт и окно проверки с домена Яндекса
		script += " " + captchaDomain
		style += " " + captchaDomain
		connect += " " + captchaDomain
		frame = "; frame-src " + captchaDomain
	}
	csp := "default-src 'self'; script-src " + script + "; style-src " + style + "; img-src " + media + " data:; media-src " + media +
		"; connect-src " + connect + frame + "; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	if s.cfg.CookieSecure {
		csp += "; upgrade-insecure-requests"
	}
	return csp
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, name string) bool {
	f, ok := s.static[name]
	if !ok {
		return false
	}
	h := w.Header()
	h.Set("Content-Type", f.ctype)
	h.Set("ETag", f.etag)
	h.Set("Content-Security-Policy", s.pageCSP())
	h.Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=(self), payment=()")
	// имена файлов без хеша — браузер переспрашивает (дёшево: 304 по ETag)
	h.Set("Cache-Control", "no-cache")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	if match := r.Header.Get("If-None-Match"); match != "" && match == f.etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	http.ServeContent(w, r, "", zeroTime, bytes.NewReader(f.data))
	return true
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if !s.serveStatic(w, r, path.Clean(r.URL.Path)) {
		http.NotFound(w, r)
	}
}

// handleSPA: любой «страничный» адрес (/u/anna, /im/12) — index.html,
// дальше маршрут разбирает фронтенд. API и файлы сюда не попадают.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	for _, prefix := range []string{"/api/", "/media/", "/audio/", "/video/", "/static/", "/docs", "/.well-known/"} {
		if strings.HasPrefix(p, prefix) {
			if strings.HasPrefix(p, "/api/") {
				writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
			} else {
				http.NotFound(w, r)
			}
			return
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	s.serveStatic(w, r, "/static/index.html")
}

var zeroTime = time.Time{}
