package httpapi

import (
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"socialnet/internal/media"
	"socialnet/internal/s3/s3test"
)

func TestMediaInS3(t *testing.T) {
	needFFmpeg(t)
	ts, srv, _ := setupFull(t, 1000)
	srv.uploadLimiter = newIPRateLimiter(100, 100)
	fake := s3test.New(t)
	srv.files = &media.S3{C: fake.Client}
	srv.avStore.Remote = fake.Client
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")

	// фото
	_, photo := upload(t, ts, anna, pngBytes(64, 48))
	pid := photo["id"].(string)
	if keys := fake.Keys(); !slices.Equal(keys, []string{"photos/" + pid + "/full", "photos/" + pid + "/thumb"}) {
		t.Fatalf("photo keys: %v", keys)
	}
	resp, err := http.Get(ts.URL + "/media/" + pid + "/thumb")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || len(body) < 100 || resp.Header.Get("Content-Type") == "" {
		t.Fatalf("photo from s3: %d %d bytes", resp.StatusCode, len(body))
	}

	// трек: на диске после обработки не остаётся ничего, отдача с Range
	_, track := uploadFile(t, ts, anna, "/api/v1/tracks", "t.mp3",
		genMedia(t, "t.mp3", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "libmp3lame"), nil)
	tid := track["id"].(string)
	if !slices.Contains(fake.Keys(), "tracks/"+tid+"/audio.mp3") {
		t.Fatalf("track not in s3: %v", fake.Keys())
	}
	if dir, _ := srv.avStore.Dir("tracks", tid); dirExists(dir) {
		t.Fatal("local copy left after commit")
	}
	req, _ := http.NewRequest("GET", ts.URL+"/audio/"+tid+".mp3", nil)
	req.Header.Set("Range", "bytes=0-9")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || len(body) != 10 || !strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes 0-9/") {
		t.Fatalf("range from s3: %d %d %q", resp.StatusCode, len(body), resp.Header.Get("Content-Range"))
	}

	// голосовое: только участникам, и никогда через CDN
	_, voice := uploadFile(t, ts, anna, "/api/v1/voice", "v.ogg",
		genMedia(t, "v.ogg", "-f", "lavfi", "-i", "sine=duration=1", "-c:a", "libopus"), nil)
	msg := anna.req("POST", p("/users/%d/messages", bob.ID), map[string]any{"attachments": []map[string]string{
		{"kind": "voice", "id": voice["id"].(string)}}}, 201)
	vurl := msg["attachments"].([]any)[0].(map[string]any)["url"].(string)
	srv.cfg.MediaCDNURL = "https://cdn.example"
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	vreq, _ := http.NewRequest("GET", ts.URL+vurl, nil)
	vreq.Header.Set("Authorization", "Bearer "+bob.Token)
	resp, _ = noRedirect.Do(vreq)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("voice via s3: %d", resp.StatusCode)
	}
	resp, _ = noRedirect.Get(ts.URL + "/audio/" + tid + ".mp3")
	resp.Body.Close()
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "https://cdn.example/tracks/"+tid+"/audio.mp3" {
		t.Fatalf("cdn redirect: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	srv.cfg.MediaCDNURL = ""

	// удаление убирает объекты из бакета
	anna.req("DELETE", p("/tracks/%s", tid), nil, 204)
	anna.req("DELETE", p("/media/%s", pid), nil, 204)
	for _, k := range fake.Keys() {
		if strings.Contains(k, tid) || strings.Contains(k, pid) {
			t.Fatalf("object left after delete: %s", k)
		}
	}
	if fake.BadSigs != 0 {
		t.Fatalf("%d requests with bad signatures", fake.BadSigs)
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
