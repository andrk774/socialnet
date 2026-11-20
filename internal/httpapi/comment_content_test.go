package httpapi

import (
	"socialnet/internal/store"
	"testing"
)

func TestCommentContent(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)
	p1 := anna.req("POST", p("/posts"), map[string]any{"text": "private", "audience": "friends"}, 201)
	pid := idOf(p1)
	code, doc := uploadFile(t, ts, bob, "/api/v1/documents", "comment.txt", []byte("private attachment"), nil)
	if code != 201 {
		t.Fatalf("upload %d %v", code, doc)
	}
	did := doc["id"].(string)
	photo, err := srv.store.CreateMedia(t.Context(), bob.ID, "image/png", 10, 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	track, video := "9cdca923-4301-40d1-b9e3-b61ee253d111", "9cdca923-4301-40d1-b9e3-b61ee253d112"
	if err = srv.store.CreateTrack(t.Context(), store.NewTrack{ID: track, Title: "Track", Artist: "Artist", UploaderID: bob.ID, DurationSec: 2, Size: 100, SourceSHA: make([]byte, 32)}); err != nil {
		t.Fatal(err)
	}
	if err = srv.store.CreateVideo(t.Context(), store.NewVideo{ID: video, Title: "Video", OwnerID: bob.ID, SourceSHA: make([]byte, 32), Format: "mp4"}); err != nil {
		t.Fatal(err)
	}
	bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"video_ids": []string{video}}, 422)
	if _, err = srv.store.VideoReady(t.Context(), video, 2, 100, 100, []string{"720p"}, "720p"); err != nil {
		t.Fatal(err)
	}
	c := bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"document_ids": []string{did}, "photo_ids": []string{photo}, "track_ids": []string{track}, "video_ids": []string{video}, "sticker": "hello"}, 201)
	cid := idOf(c)
	anna.req("POST", p("/posts/%d/comments", pid), map[string]any{"document_ids": []string{did}}, 422)
	bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"sticker": "fake"}, 422)
	bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"photo_ids": []string{"bad"}}, 422)
	bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": ""}, 422)
	for _, reaction := range []string{"like", "like", "love"} {
		anna.req("POST", p("/comments/%d/reactions", cid), map[string]any{"reaction": reaction}, 204)
	}
	got := items(anna.req("GET", p("/posts/%d/comments", pid), nil, 200))[0].(map[string]any)
	if got["sticker"] != "hello" || got["likes_count"] != float64(1) || got["my_reaction"] != "love" || len(got["documents"].([]any)) != 1 {
		t.Fatalf("rich comment: %v", got)
	}
	for _, key := range []string{"photos", "tracks", "videos"} {
		if len(got[key].([]any)) != 1 {
			t.Fatalf("missing %s: %v", key, got)
		}
	}
	if got["tracks"].([]any)[0].(map[string]any)["url"] != store.AudioURL(track) {
		t.Fatal("incorrect audio URL")
	}
	anna.req("POST", p("/posts/%d/comments", pid), map[string]any{"photo_ids": []string{photo}}, 422)
	reactors := items(bob.req("GET", p("/comments/%d/reactions", cid), nil, 200))
	if len(reactors) != 1 || reactors[0].(map[string]any)["user"].(map[string]any)["id"] != float64(anna.ID) {
		t.Fatalf("reactors: %v", reactors)
	}
	eve.req("GET", p("/comments/%d/reactions", cid), nil, 404)
	eve.req("POST", p("/comments/%d/reactions", cid), map[string]any{"reaction": "like"}, 404)
	eve.req("DELETE", p("/comments/%d/reactions", cid), nil, 404)
	if status, _, _ := download(t, ts, anna, doc["url"].(string)); status != 200 {
		t.Fatalf("visible attachment: %d", status)
	}
	if status, _, _ := download(t, ts, eve, doc["url"].(string)); status != 404 {
		t.Fatalf("private attachment leaked: %d", status)
	}
	anna.req("DELETE", p("/comments/%d/reactions", cid), nil, 204)
	if len(items(bob.req("GET", p("/comments/%d/reactions", cid), nil, 200))) != 0 {
		t.Fatal("reaction not removed")
	}
	bob.req("DELETE", p("/comments/%d", cid), nil, 204)
	anna.req("GET", p("/comments/%d/reactions", cid), nil, 404)
	if status, _, _ := download(t, ts, anna, doc["url"].(string)); status != 404 {
		t.Fatalf("deleted comment attachment leaked: %d", status)
	}
}
