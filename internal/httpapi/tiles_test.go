package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var pngTile = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)

func TestTileProxy(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	var hits atomic.Int32
	var gotUA atomic.Value
	var mode atomic.Value
	mode.Store("ok")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotUA.Store(r.UserAgent())
		time.Sleep(50 * time.Millisecond) // чтобы одновременные запросы застали друг друга
		switch mode.Load() {
		case "html":
			_, _ = w.Write([]byte("<html><script>alert(1)</script></html>"))
		case "down":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write(pngTile)
		}
	}))
	defer upstream.Close()

	get := func(path string) (int, http.Header, []byte) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, b
	}

	// карта выключена — тайлов нет, клиент об этом знает
	if code, _, _ := get("/tiles/1/0/0.png"); code != 404 {
		t.Fatalf("disabled map served a tile: %d", code)
	}
	c := newClient(t, ts.URL)
	if code, cfg := c.do("GET", "/api/v1/client-config", "", nil); code != 200 || cfg["map"].(map[string]any)["enabled"] != false {
		t.Fatalf("client-config: %d %v", code, cfg)
	}

	srv.cfg.TileURL = upstream.URL + "/{z}/{x}/{y}.png"
	srv.cfg.TileMaxZoom = 17
	srv.cfg.AppURL = "https://hike.example"
	srv.tiles = newTileProxy(t.TempDir())
	srv.tileLimiter = newIPRateLimiter(1000, 1000)

	// одновременно 10 посетителей — к источнику один запрос
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, h, b := get("/tiles/12/2780/1268.png")
			if code != 200 || h.Get("Content-Type") != "image/png" || len(b) != len(pngTile) {
				t.Errorf("tile: %d %v", code, h)
			}
		}()
	}
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("upstream hit %d times for one tile", n)
	}
	if ua := gotUA.Load().(string); ua != "socialnet/1.0 (+https://hike.example; security@localhost)" && ua != "socialnet/1.0 (+https://hike.example; )" {
		t.Errorf("User-Agent for the tile source: %q", ua)
	}
	// из кеша — без запроса к источнику
	get("/tiles/12/2780/1268.png")
	if n := hits.Load(); n != 1 {
		t.Fatalf("cached tile fetched again: %d", n)
	}
	_, h, _ := get("/tiles/12/2780/1268.png")
	if h.Get("Cache-Control") != "public, max-age=86400" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", h)
	}

	// неверные координаты не доходят до источника
	for _, p := range []string{"/tiles/18/0/0.png", "/tiles/-1/0/0.png", "/tiles/3/8/0.png", "/tiles/3/0/8.png", "/tiles/a/b/c.png", "/tiles/3/../0.png"} {
		if code, _, _ := get(p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("invalid tile reached upstream: %d", n)
	}

	// источник отдал не картинку — не кешируем и не пересылаем
	mode.Store("html")
	if code, _, b := get("/tiles/5/1/1.png"); code != 502 || len(b) > 100 && string(b[:6]) == "<html>" {
		t.Errorf("non-image tile passed through: %d", code)
	}

	// источник лёг — устаревший тайл лучше дыры
	mode.Store("down")
	old := time.Now().Add(-8 * 24 * time.Hour)
	p := srv.tiles.path(12, 2780, 1268)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if code, _, b := get("/tiles/12/2780/1268.png"); code != 200 || len(b) != len(pngTile) {
		t.Errorf("stale tile not served when source is down: %d", code)
	}
	if code, _, _ := get("/tiles/6/1/1.png"); code != 502 {
		t.Errorf("missing tile with source down: %d", code)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "..", "..", "5", "1", "1.tile")); err == nil {
		t.Error("bad tile was cached")
	}

	if code, cfg := c.do("GET", "/api/v1/client-config", "", nil); code != 200 || cfg["map"].(map[string]any)["enabled"] != true {
		t.Fatalf("client-config enabled: %v", cfg)
	}
}
