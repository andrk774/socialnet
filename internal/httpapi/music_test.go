package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"socialnet/internal/av"
)

func needFFmpeg(t *testing.T) {
	if !av.Available() {
		t.Skip("ffmpeg not installed")
	}
}

// genMedia создаёт тестовый файл генераторами ffmpeg.
func genMedia(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), name)
	full := append([]string{"-hide_banner", "-v", "error", "-y"}, args...)
	if b, err := exec.Command("ffmpeg", append(full, out)...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, b)
	}
	b, _ := os.ReadFile(out)
	return b
}

func uploadFile(t *testing.T, ts *httptest.Server, u *tuser, path, filename string, data []byte, fields map[string]string) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+u.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestMusic(t *testing.T) {
	needFFmpeg(t)
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	anna, bob, mod := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "moder")
	_ = srv.store.SetRole(t.Context(), mod.ID, "moderator")

	song := genMedia(t, "song.mp3", "-f", "lavfi", "-i", "sine=frequency=440:duration=4",
		"-metadata", "title=Уральская", "-metadata", "artist=Миасс Бэнд", "-c:a", "libmp3lame")

	code, tr := uploadFile(t, ts, anna, "/api/v1/tracks", "song.mp3", song, nil)
	if code != 201 || tr["title"] != "Уральская" || tr["artist"] != "Миасс Бэнд" || tr["in_my_music"] != true {
		t.Fatalf("upload: %d %v", code, tr)
	}
	tid := tr["id"].(string)

	// поля формы важнее тегов файла; управляющие символы вырезаются
	code, tr2 := uploadFile(t, ts, anna, "/api/v1/tracks", "x.mp3",
		genMedia(t, "b.ogg", "-f", "lavfi", "-i", "sine=frequency=300:duration=2", "-c:a", "libopus"),
		map[string]string{"title": "Песня‮\nвторая", "artist": "Анна"})
	if code != 201 || tr2["title"] != "Песня вторая" {
		t.Fatalf("ogg upload: %d %v", code, tr2)
	}

	// прослушивание с перемоткой (Range)
	req, _ := http.NewRequest("GET", ts.URL+tr["url"].(string), nil)
	req.Header.Set("Range", "bytes=100-199")
	resp, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || len(body) != 100 || resp.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("range: %d len=%d %v", resp.StatusCode, len(body), resp.Header)
	}

	// плохие файлы
	evil := []byte("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:10.0,\nfile:///etc/passwd\n#EXT-X-ENDLIST\n")
	if code, _ := uploadFile(t, ts, anna, "/api/v1/tracks", "evil.mp3", evil, nil); code != 415 {
		t.Fatalf("HLS playlist disguised as mp3 accepted: %d", code)
	}
	if code, _ := uploadFile(t, ts, anna, "/api/v1/tracks", "a.mp3", []byte("not audio at all"), nil); code != 415 {
		t.Fatalf("text accepted: %d", code)
	}
	silentVideo := genMedia(t, "v.mp4", "-f", "lavfi", "-i", "testsrc=size=160x120:duration=2", "-c:v", "libx264", "-preset", "ultrafast")
	if code, _ := uploadFile(t, ts, anna, "/api/v1/tracks", "v.mp3", silentVideo, nil); code != 422 {
		t.Fatalf("video without audio accepted as track: %d", code)
	}

	// «Моя музыка»: Боб добавляет трек Анны
	bob.req("POST", p("/tracks/%s/my-music", tid), nil, 204)
	if n := len(items(bob.req("GET", p("/users/%d/music", bob.ID), nil, 200))); n != 1 {
		t.Fatalf("bob music: %d", n)
	}
	if n := len(items(bob.req("GET", p("/search?type=tracks&q=уральская"), nil, 200))); n != 1 {
		t.Fatalf("track search: %d", n)
	}

	// плейлист: правит только владелец
	pl := bob.req("POST", p("/playlists"), map[string]any{"title": "Для пробежки"}, 201)
	plid := int64(pl["id"].(float64))
	bob.req("POST", p("/playlists/%d/tracks", plid), map[string]any{"track_id": tid}, 204)
	bob.req("POST", p("/playlists/%d/tracks", plid), map[string]any{"track_id": "00000000-0000-4000-8000-000000000000"}, 404)
	anna.req("POST", p("/playlists/%d/tracks", plid), map[string]any{"track_id": tid}, 403)
	anna.req("DELETE", p("/playlists/%d", plid), nil, 403)
	if got := anna.req("GET", p("/playlists/%d", plid), nil, 200); len(got["tracks"].([]any)) != 1 {
		t.Fatalf("playlist: %v", got)
	}
	bob.req("PATCH", p("/users/me/privacy"), map[string]any{"profile_visibility": "only_me"}, 200)
	anna.req("GET", p("/playlists/%d", plid), nil, 404) // закрытый профиль
	bob.req("PATCH", p("/users/me/privacy"), map[string]any{"profile_visibility": "everyone"}, 200)

	// трек в посте
	post := bob.req("POST", p("/posts"), map[string]any{"text": "слушайте", "track_ids": []string{tid}}, 201)
	pid := int64(post["id"].(float64))
	if tracks := post["tracks"].([]any); len(tracks) != 1 || tracks[0].(map[string]any)["url"] == nil {
		t.Fatalf("post tracks: %v", post)
	}

	// чужой трек удалить нельзя
	bob.req("DELETE", p("/tracks/%s", tid), nil, 403)

	// жалоба правообладателя → модератор снимает трек
	rep := bob.req("POST", p("/reports"), map[string]any{"target_type": "track", "target_uuid": tid, "reason": "нарушение авторских прав"}, 201)
	bob.req("POST", p("/reports"), map[string]any{"target_type": "track", "target_uuid": "bad", "reason": "x"}, 422)
	mod.req("POST", p("/mod/reports/%d/resolve", int64(rep["id"].(float64))), map[string]any{"action": "hide", "note": "DMCA"}, 204)

	if r, _ := http.Get(ts.URL + tr["url"].(string)); r.StatusCode != 404 {
		t.Fatal("removed track still streams")
	}
	if got := bob.req("GET", p("/posts/%d", pid), nil, 200); len(got["tracks"].([]any)) != 0 {
		t.Fatal("removed track still in post")
	}
	if n := len(items(bob.req("GET", p("/users/%d/music", bob.ID), nil, 200))); n != 0 {
		t.Fatal("removed track still in my music")
	}
	// тот же файл перезалить нельзя
	if code, _ := uploadFile(t, ts, bob, "/api/v1/tracks", "reupload.mp3", song, nil); code != 451 {
		t.Fatalf("blocked file re-uploaded: %d", code)
	}

	// восстановление снимает блокировку
	mod.req("POST", p("/mod/tracks/%s/unhide", tid), nil, 204)
	if r, _ := http.Get(ts.URL + tr["url"].(string)); r.StatusCode != 200 {
		t.Fatal("restored track not streaming")
	}
	anna.req("DELETE", p("/tracks/%s", tid), nil, 204)
	anna.req("GET", p("/tracks/%s", tid), nil, 404)
}
