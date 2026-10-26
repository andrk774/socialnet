package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"socialnet/internal/av"
	"socialnet/internal/worker"
)

func TestFrontendServing(t *testing.T) {
	ts := setup(t, 1000)
	for _, path := range []string{"/", "/u/anna", "/im/5", "/oauth/authorize?client_id=x"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		csp := resp.Header.Get("Content-Security-Policy")
		if resp.StatusCode != 200 || !strings.Contains(string(body), `id="app"`) ||
			!strings.Contains(csp, "script-src 'self';") || strings.Contains(csp, "unsafe") {
			t.Fatalf("%s: %d csp=%q", path, resp.StatusCode, csp)
		}
	}
	resp, _ := http.Get(ts.URL + "/static/app/main.js")
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") || etag == "" {
		t.Fatalf("main.js: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	req, _ := http.NewRequest("GET", ts.URL+"/static/app/main.js", nil)
	req.Header.Set("If-None-Match", etag)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 304 {
		t.Fatalf("etag revalidation: %d", resp.StatusCode)
	}
	// API не превращается в index.html, исходники и типы не отдаются
	for path, want := range map[string]int{"/api/v1/nope": 404, "/static/../go.mod": 404, "/static/vendor/preact/index.d.ts": 404, "/static/nope.js": 404} {
		resp, _ := http.Get(ts.URL + path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want || strings.Contains(string(body), `id="app"`) {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
}

// TestFrontendE2E — сценарий в Chromium (Playwright). Нужны node и playwright.
func TestFrontendE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	if exec.Command(node, "-e", "require('playwright')").Run() != nil {
		t.Skip("playwright not installed")
	}
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	// видео для проверки плеера: воркер обрабатывает очередь в фоне
	var clip string
	if av.Available() {
		data := genMedia(t, "clip.mp4", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=24:duration=4",
			"-f", "lavfi", "-i", "sine=duration=4", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac")
		clip = filepath.Join(t.TempDir(), "clip.mp4")
		if err := os.WriteFile(clip, data, 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go (&worker.Worker{Store: srv.store, AV: srv.avStore, Log: slog.New(&failOnError{t: t})}).Run(ctx)
	}
	// Анна — администратор: роль читается из базы на каждом запросе,
	// поэтому достаточно выставить её, как только Анна зарегистрируется.
	adminCtx, stopAdmin := context.WithCancel(context.Background())
	t.Cleanup(stopAdmin)
	go func() {
		for adminCtx.Err() == nil {
			tag, err := srv.store.Pool().Exec(adminCtx, `UPDATE users SET role = 'admin' WHERE username = 'anna' AND role = 'user'`)
			if err == nil && tag.RowsAffected() > 0 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	_, file, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(file), "../../web/e2e/smoke.mjs")
	shots := os.Getenv("E2E_SCREENSHOTS")
	if shots == "" {
		shots = t.TempDir()
	}
	cmd := exec.Command(node, script, ts.URL, shots, clip)
	// LANG: без UTF-8-локали Chromium в Linux заменяет кириллическое имя скачиваемого файла на «download»
	cmd.Env = append(os.Environ(), "NODE_PATH="+os.Getenv("NODE_PATH"), "LANG=C.UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
