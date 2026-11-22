package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Прокси тайлов карты с кешем на диске.
//
// Зачем свой прокси, а не тайлы OpenStreetMap напрямую из браузера:
//   - IP и адреса страниц посетителей не уходят на чужой сервер;
//   - CSP остаётся «img-src 'self'»;
//   - правила OSM для сайтов — свой User-Agent с контактом и кеш не меньше
//     недели — выполняются в одном месте, а повторные запросы не доходят до OSM.
// Для заметного трафика правила OSM просят свой тайл-сервер или платного
// поставщика: тогда меняется только TILE_URL.

const (
	tileTTL        = 7 * 24 * time.Hour
	tileMaxBytes   = 1 << 20
	tileFetchLimit = 4 // одновременных запросов к источнику
)

type tileProxy struct {
	client   *http.Client
	dir      string
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[string]*tileCall
}

type tileCall struct {
	done chan struct{}
	data []byte
	err  error
}

func newTileProxy(dir string) *tileProxy {
	return &tileProxy{
		client:   &http.Client{Timeout: 10 * time.Second},
		dir:      dir,
		sem:      make(chan struct{}, tileFetchLimit),
		inflight: map[string]*tileCall{},
	}
}

// tileKind по сигнатуре: только PNG и JPEG, остальное источнику не простим.
func tileKind(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	}
	return ""
}

func (s *Server) handleTile(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TileURL == "" || s.tiles == nil {
		http.NotFound(w, r)
		return
	}
	z, err1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, err2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, err3 := strconv.Atoi(strings.TrimSuffix(chi.URLParam(r, "y"), ".png"))
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > s.cfg.TileMaxZoom || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		http.NotFound(w, r)
		return
	}
	if !s.tileLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many map tiles")
		return
	}
	data, err := s.tiles.get(r.Context(), s.cfg.TileURL, s.tileUserAgent(), z, x, y)
	if err != nil {
		if r.Context().Err() == nil {
			s.log.Warn("map tile", "err", err, "z", z, "x", x, "y", y)
		}
		http.Error(w, "tile unavailable", http.StatusBadGateway)
		return
	}
	h := w.Header()
	h.Set("Content-Type", tileKind(data))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func (s *Server) tileUserAgent() string {
	return "socialnet/1.0 (+" + s.cfg.AppURL + "; " + s.cfg.SecurityContact + ")"
}

func (t *tileProxy) path(z, x, y int) string {
	return filepath.Join(t.dir, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".tile")
}

// get: свежий тайл из кеша, иначе — один запрос к источнику на тайл, сколько бы
// посетителей его ни ждали. Если источник недоступен, отдаём устаревший кеш.
func (t *tileProxy) get(ctx context.Context, tmpl, ua string, z, x, y int) ([]byte, error) {
	p := t.path(z, x, y)
	cached, err := os.ReadFile(p) // #nosec G304 -- путь из проверенных чисел z/x/y
	if err == nil {
		if st, serr := os.Stat(p); serr == nil && time.Since(st.ModTime()) < tileTTL {
			return cached, nil
		}
	}
	key := p
	t.mu.Lock()
	call, ok := t.inflight[key]
	if !ok {
		call = &tileCall{done: make(chan struct{})}
		t.inflight[key] = call
		t.mu.Unlock()
		go func() {
			// без отмены от запроса: тайл доводится до кеша, даже если первый посетитель ушёл
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			call.data, call.err = t.fetch(fctx, tmpl, ua, z, x, y)
			if call.err == nil {
				_ = t.save(p, call.data)
			}
			t.mu.Lock()
			delete(t.inflight, key)
			t.mu.Unlock()
			close(call.done)
		}()
	} else {
		t.mu.Unlock()
	}
	select {
	case <-call.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if call.err != nil && cached != nil {
		return cached, nil // источник лежит — лучше вчерашний тайл, чем дыра в карте
	}
	return call.data, call.err
}

func (t *tileProxy) fetch(ctx context.Context, tmpl, ua string, z, x, y int) ([]byte, error) {
	select {
	case t.sem <- struct{}{}:
		defer func() { <-t.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	u := strings.NewReplacer("{z}", strconv.Itoa(z), "{x}", strconv.Itoa(x), "{y}", strconv.Itoa(y)).Replace(tmpl)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil) // #nosec G704 -- адрес из TILE_URL администратора, z/x/y — проверенные числа
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := t.client.Do(req) // #nosec G704 -- адрес из TILE_URL администратора, z/x/y — проверенные числа
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile source: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, tileMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > tileMaxBytes {
		return nil, errors.New("tile source: tile too large")
	}
	if tileKind(data) == "" {
		return nil, errors.New("tile source: not a PNG/JPEG image")
	}
	return data, nil
}

func (t *tileProxy) save(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tile-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// handleClientConfig: GET /api/v1/client-config — что включено на сервере.
func (s *Server) handleClientConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"map": map[string]any{
			"enabled":     s.cfg.TileURL != "",
			"tiles":       "/tiles/{z}/{x}/{y}.png",
			"attribution": s.cfg.TileAttribution,
			"max_zoom":    s.cfg.TileMaxZoom,
		},
		"weather": map[string]any{"enabled": s.cfg.WeatherURL != ""},
	})
}
