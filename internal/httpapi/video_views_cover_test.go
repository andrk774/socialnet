package httpapi

import (
	"fmt"
	"socialnet/internal/store"
	"testing"
)

func TestVideoViewsAndCover(t *testing.T) {
	s, _, a, b, _ := setupWallet(t)
	ctx := t.Context()
	id, e := s.store.CreateMedia(ctx, a.ID, "image/png", 128, 128, 100)
	if e != nil {
		t.Fatal(e)
	}
	b.req("PUT", p("/users/me/cover"), map[string]any{"media_id": id}, 404)
	a.req("PUT", p("/users/me/cover"), map[string]any{"media_id": id}, 204)
	got := a.req("GET", p(fmt.Sprintf("/users/%d/cover", a.ID)), nil, 200)
	if got["url"] != "/media/"+id+"/full" {
		t.Fatal(got)
	}
	a.req("PUT", p("/users/me/cover"), map[string]any{"media_id": nil}, 204)
	vid := "11111111-1111-4111-8111-111111111111"
	e = s.store.CreateVideo(ctx, store.NewVideo{ID: vid, OwnerID: a.ID, Title: "Test", SourceSHA: make([]byte, 32), Format: "mp4"})
	if e != nil {
		t.Fatal(e)
	}
	a.req("POST", p("/videos/"+vid+"/view"), nil, 409)
	if _, e = s.store.Pool().Exec(ctx, `UPDATE videos SET status='ready' WHERE id=$1`, vid); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		got = a.req("POST", p("/videos/"+vid+"/view"), nil, 200)
		if got["views"] != float64(1) {
			t.Fatal(got)
		}
	}
	got = a.req("GET", p("/videos/"+vid), nil, 200)
	if got["views"] != float64(1) {
		t.Fatal(got)
	}
}
