package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socialnet/internal/av"
	"socialnet/internal/worker"
)

// checkMP4: MP4 отдаётся с Range и это настоящее видео со звуком нужной длины.
func checkMP4(t *testing.T, base string, v map[string]any, vid string) {
	t.Helper()
	if v["mp4_url"] != "/video/"+vid+"/720p.mp4" {
		t.Fatalf("mp4_url: %v", v["mp4_url"])
	}
	req, _ := http.NewRequest("GET", base+v["mp4_url"].(string), nil)
	req.Header.Set("Range", "bytes=0-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 206 || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("mp4 range: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp, err = http.Get(base + v["mp4_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	f := filepath.Join(t.TempDir(), "v.mp4")
	if err := os.WriteFile(f, data, 0o600); err != nil {
		t.Fatal(err)
	}
	probe, err := av.ProbeFile(t.Context(), f)
	if err != nil || probe.Video() == nil || probe.Audio() == nil || probe.Seconds < 2.5 || probe.Seconds > 3.5 || probe.Video().Height != 720 {
		t.Fatalf("mp4 probe: %+v %v", probe, err)
	}
	// faststart: индекс (moov) до данных (mdat), видео начинает играть сразу
	if moov, mdat := strings.Index(string(data[:4096]), "moov"), strings.Index(string(data), "mdat"); moov < 0 || moov > mdat {
		t.Fatalf("moov at %d, mdat at %d", moov, mdat)
	}
}

func TestVideo(t *testing.T) {
	needFFmpeg(t)
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	wk := &worker.Worker{Store: srv.store, AV: srv.avStore, Log: slog.New(&failOnError{t: t})}
	anna, bob, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")

	clip := genMedia(t, "clip.mp4", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=24:duration=3",
		"-f", "lavfi", "-i", "sine=duration=3", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-metadata", "location=+55.0415+060.1077/")

	code, v := uploadFile(t, ts, anna, "/api/v1/videos", "Закат над Ильменским.mp4", clip, map[string]string{"description": "озеро"})
	if code != 202 || v["status"] != "processing" || v["title"] != "Закат над Ильменским" {
		t.Fatalf("upload: %d %v", code, v)
	}
	vid := v["id"].(string)

	// пока обрабатывается — видно только владельцу, к постам не прикрепить
	bob.req("GET", p("/videos/%s", vid), nil, 404)
	anna.req("GET", p("/videos/%s", vid), nil, 200)
	anna.req("POST", p("/posts"), map[string]any{"video_ids": []string{vid}}, 422)

	// воркер обрабатывает очередь
	if did, err := wk.ProcessOne(t.Context()); !did || err != nil {
		t.Fatalf("worker: %v %v", did, err)
	}
	if did, _ := wk.ProcessOne(t.Context()); did {
		t.Fatal("queue should be empty")
	}

	v = bob.req("GET", p("/videos/%s", vid), nil, 200)
	if v["status"] != "ready" || len(v["qualities"].([]any)) != 2 || v["duration_sec"].(float64) != 3 {
		t.Fatalf("ready video: %v", v)
	}

	get := func(path string) (*http.Response, string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(b)
	}
	resp, master := get(v["hls_url"].(string))
	if resp.StatusCode != 200 || !strings.Contains(master, "720p.m3u8") ||
		resp.Header.Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("master: %d %s", resp.StatusCode, master)
	}
	_, playlist := get("/video/" + vid + "/360p.m3u8")
	if !strings.Contains(playlist, "360p_0000.ts") {
		t.Fatalf("playlist: %s", playlist)
	}
	if resp, seg := get("/video/" + vid + "/360p_0000.ts"); resp.StatusCode != 200 || len(seg) < 1000 ||
		strings.Contains(seg, "55.0415") {
		t.Fatalf("segment: %d len=%d", resp.StatusCode, len(seg))
	}
	if resp, _ := get(v["poster_url"].(string)); resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatal("poster")
	}
	checkMP4(t, ts.URL, v, vid)

	for _, bad := range []string{"../../../etc/passwd", "secret.txt", "360p_0000.ts.bak"} {
		if resp, _ := get("/video/" + vid + "/" + bad); resp.StatusCode != 404 {
			t.Fatalf("served %s", bad)
		}
	}

	// владелец получил уведомление
	if n := items(anna.req("GET", p("/notifications"), nil, 200))[0].(map[string]any); n["type"] != "video_ready" {
		t.Fatalf("notification: %v", n)
	}

	// в пост, правка, видимость
	post := bob.req("POST", p("/posts"), map[string]any{"text": "смотрите", "video_ids": []string{vid}}, 201)
	if vs := post["videos"].([]any); len(vs) != 1 || vs[0].(map[string]any)["hls_url"] == nil ||
		vs[0].(map[string]any)["mp4_url"] != "/video/"+vid+"/720p.mp4" {
		t.Fatalf("post videos: %v", post)
	}
	// видео, обработанное до появления MP4: команда backfill-mp4 досоздаёт его из сегментов
	mp4Path, _ := srv.avStore.Path("videos", vid, "720p.mp4")
	if err := os.Remove(mp4Path); err != nil {
		t.Fatal(err)
	}
	srv.store.Pool().Exec(t.Context(), `UPDATE videos SET mp4 = NULL`) //nolint:errcheck
	if v := anna.req("GET", p("/videos/%s", vid), nil, 200); v["mp4_url"] != nil {
		t.Fatalf("mp4_url without file: %v", v)
	}
	if n, err := srv.store.EnqueueMP4Backfill(t.Context()); err != nil || n != 1 {
		t.Fatalf("backfill queued %d %v", n, err)
	}
	if n, _ := srv.store.EnqueueMP4Backfill(t.Context()); n != 0 {
		t.Fatal("backfill job duplicated")
	}
	if did, err := wk.ProcessOne(t.Context()); !did || err != nil {
		t.Fatalf("backfill worker: %v %v", did, err)
	}
	checkMP4(t, ts.URL, anna.req("GET", p("/videos/%s", vid), nil, 200), vid)

	bob.req("PATCH", p("/videos/%s", vid), map[string]any{"title": "моё"}, 403)
	anna.req("PATCH", p("/videos/%s", vid), map[string]any{"title": "Закат"}, 200)
	if n := len(bob.req("GET", p("/users/%d/videos", anna.ID), nil, 200)["items"].([]any)); n != 1 {
		t.Fatalf("user videos: %d", n)
	}

	// плохие файлы
	if code, _ := uploadFile(t, ts, anna, "/api/v1/videos", "x.mp4", []byte("#EXTM3U\nfile:///etc/passwd"), nil); code != 415 {
		t.Fatalf("playlist accepted: %d", code)
	}
	audioOnly := genMedia(t, "a.m4a", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "aac")
	if code, _ := uploadFile(t, ts, anna, "/api/v1/videos", "a.mp4", audioOnly, nil); code != 422 {
		t.Fatalf("audio-only accepted as video: %d", code)
	}

	// удаление во время обработки: воркер не падает и не оставляет файлов
	code, v2 := uploadFile(t, ts, anna, "/api/v1/videos", "second.mp4",
		genMedia(t, "c2.mp4", "-f", "lavfi", "-i", "testsrc=size=320x240:duration=2", "-c:v", "libx264", "-preset", "ultrafast"), nil)
	if code != 202 {
		t.Fatalf("second upload: %d", code)
	}
	anna.req("DELETE", p("/videos/%s", v2["id"].(string)), nil, 204)
	if _, err := wk.ProcessOne(t.Context()); err != nil {
		t.Fatalf("worker after delete: %v", err)
	}

	// жалоба → снятие → не отдаётся и не перезаливается
	rep := bob.req("POST", p("/reports"), map[string]any{"target_type": "video", "target_uuid": vid, "reason": "чужое видео"}, 201)
	mod.req("POST", p("/mod/reports/%d/resolve", int64(rep["id"].(float64))), map[string]any{"action": "hide"}, 204)
	if resp, _ := get("/video/" + vid + "/master.m3u8"); resp.StatusCode != 404 {
		t.Fatal("removed video still served")
	}
	bob.req("GET", p("/videos/%s", vid), nil, 404)
	if code, _ := uploadFile(t, ts, bob, "/api/v1/videos", "copy.mp4", clip, nil); code != 451 {
		t.Fatalf("removed video re-uploaded: %d", code)
	}
}
