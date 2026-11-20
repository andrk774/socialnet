package httpapi

import "testing"

func TestPhotoComments(t *testing.T) {
	ts, s, _ := setupFull(t, 1000)
	a, b := register(t, ts, "anna"), register(t, ts, "bob")
	id, err := s.store.CreateMedia(t.Context(), a.ID, "image/png", 10, 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	url := "/photos/" + id + "/comments"
	b.req("GET", p(url), nil, 404)
	a.req("POST", p(url), map[string]any{"text": "   "}, 422)
	a.req("POST", p(url), map[string]any{"text": "Hello"}, 201)
	got := a.req("GET", p(url), nil, 200)
	if len(got["items"].([]any)) != 1 {
		t.Fatal(got)
	}
	a.req("POST", p("/posts"), map[string]any{"text": "photo", "photo_ids": []string{id}, "audience": "everyone"}, 201)
	b.req("POST", p(url), map[string]any{"text": "Nice"}, 201)
	b.req("GET", p(url+"?after=bad"), nil, 400)
}
