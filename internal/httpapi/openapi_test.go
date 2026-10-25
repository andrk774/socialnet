package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"socialnet/api"
	"socialnet/internal/config"
)

// TestOpenAPICoversRoutes: каждый маршрут описан в спецификации, и в ней
// нет несуществующих маршрутов. Добавили эндпоинт — обновите api/gen_openapi.py.
func TestOpenAPICoversRoutes(t *testing.T) {
	srv, err := New(config.Config{JWTSecret: []byte(strings.Repeat("s", 32)),
		TOTPKey: []byte(strings.Repeat("t", 32)), MediaDir: t.TempDir()}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	_ = chi.Walk(srv.Router().(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/docs") || strings.HasPrefix(route, "/static") { // документация и файлы фронтенда — не API
			return nil
		}
		routes[method+" "+strings.ReplaceAll(route, "/*", "")] = true
		return nil
	})

	raw, _ := api.Files.ReadFile("openapi.json")
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for m := range ops {
			documented[strings.ToUpper(m)+" "+path] = true
		}
	}
	var missing, stale []string
	for r := range routes {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		if !routes[d] {
			stale = append(stale, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("openapi.json out of sync (run python3 api/gen_openapi.py)\nundocumented: %v\nstale: %v", missing, stale)
	}
}

func TestDocsPage(t *testing.T) {
	ts := setup(t, 1000)
	for path, ctype := range map[string]string{"/docs/": "text/html", "/docs/swagger-ui-bundle.js": "javascript", "/api/v1/openapi.json": "application/json"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), ctype) {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		if strings.HasPrefix(path, "/docs") && strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src 'self' 'unsafe-inline'") {
			t.Fatal("docs CSP allows inline scripts")
		}
	}
	_ = httptest.NewRecorder
}
