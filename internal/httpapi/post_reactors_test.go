package httpapi

import "testing"

func TestPostReactors(t *testing.T) {
	ts := setup(t, 1000)
	a, b := register(t, ts, "reactora"), register(t, ts, "reactorb")
	created := a.req("POST", p("/posts"), map[string]any{"text": "reactors"}, 201)
	id := int64(created["id"].(float64))
	b.req("POST", p("/posts/%d/reactions", id), map[string]any{"reaction": "love"}, 200)
	a.req("POST", p("/posts/%d/reactions", id), map[string]any{"reaction": "like"}, 200)
	first := a.req("GET", p("/posts/%d/reactions?limit=1", id), nil, 200)
	if len(items(first)) != 1 {
		t.Fatal(first)
	}
	second := a.req("GET", p("/posts/%d/reactions?after=%d", id, int64(first["next_cursor"].(float64))), nil, 200)
	if len(items(second)) != 1 || items(second)[0].(map[string]any)["reaction"] != "love" {
		t.Fatal(second)
	}
	b.req("DELETE", p("/posts/%d/reactions", id), nil, 200)
	if len(items(a.req("GET", p("/posts/%d/reactions", id), nil, 200))) != 1 {
		t.Fatal("removed reaction still listed")
	}
	a.req("PATCH", p("/posts/%d", id), map[string]any{"audience": "only_me"}, 200)
	b.req("GET", p("/posts/%d/reactions", id), nil, 404)
}
