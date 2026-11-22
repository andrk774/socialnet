package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPrivateVideoMessages(t *testing.T) {
	needFFmpeg(t)
	ts, _, _ := setupFull(t, 1000)
	a, b, c := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	video := genMedia(t, "note.mp4", "-f", "lavfi", "-i", "color=c=blue:s=320x240:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	code, v := uploadFile(t, ts, a, "/api/v1/video-messages", "note.mp4", video, nil)
	if code != 201 {
		t.Fatalf("upload %d %v", code, v)
	}
	id := v["id"].(string)
	url := v["url"].(string)
	check := func(u *tuser, status int) {
		t.Helper()
		r, _ := http.NewRequest("GET", ts.URL+url, nil)
		r.Header.Set("Authorization", "Bearer "+u.Token)
		r.Header.Set("Range", "bytes=0-31")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != status {
			t.Fatalf("GET %d want %d: %s", resp.StatusCode, status, data)
		}
		if status == 206 && (!strings.Contains(resp.Header.Get("Cache-Control"), "private") || len(data) != 32 || resp.Header.Get("Content-Type") != "video/mp4") {
			t.Fatal("invalid private video response")
		}
	}
	check(a, 206)
	check(b, 404)
	b.req("POST", p("/users/%d/messages", a.ID), map[string]any{"attachments": []any{map[string]string{"kind": "video", "id": id}}}, 422)
	m := a.req("POST", p("/users/%d/messages", b.ID), map[string]any{"attachments": []any{map[string]string{"kind": "video", "id": id}}}, 201)
	att := m["attachments"].([]any)[0].(map[string]any)
	if att["kind"] != "video" || att["url"] != url {
		t.Fatal(att)
	}
	check(b, 206)
	check(c, 404)
	a.req("DELETE", p("/conversations/%d/messages/%d", int64(m["conversation_id"].(float64)), int64(m["id"].(float64))), nil, 200)
	check(b, 404)
}
