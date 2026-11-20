package httpapi

import "testing"

func TestPhotoLibrary(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	_, photo := upload(t, ts, anna, pngBytes(30, 20))
	id := photo["id"].(string)
	anna.req("PATCH", p("/photos/%s", id), map[string]any{"description": "Saved caption"}, 204)
	bob.req("PATCH", p("/photos/%s", id), map[string]any{"description": "Not allowed"}, 404)
	caption := anna.req("GET", p("/photos"), nil, 200)["items"].([]any)[0].(map[string]any)["description"]
	if caption != "Saved caption" {
		t.Fatal("description persistence or ownership failed")
	}
	_, other := upload(t, ts, bob, pngBytes(30, 20))
	otherID := other["id"].(string)
	album := anna.req("POST", p("/photo-albums"), map[string]any{"title": "Trip"}, 201)
	foreign := bob.req("POST", p("/photo-albums"), map[string]any{"title": "Private"}, 201)
	bob.req("PATCH", p("/photos"), map[string]any{"ids": []string{id}, "album_id": foreign["id"]}, 404)
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id}, "album_id": foreign["id"]}, 404)
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id, otherID}, "archived": true}, 404)
	own := anna.req("GET", p("/photos"), nil, 200)["items"].([]any)
	if len(own) != 1 || own[0].(map[string]any)["archived"] != false {
		t.Fatal("ownership or transaction rollback failed")
	}
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id}, "album_id": album["id"]}, 204)
	items := anna.req("GET", p("/photo-albums"), nil, 200)["items"].([]any)
	if items[0].(map[string]any)["count"] != float64(1) {
		t.Fatal("album count")
	}
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id}, "archived": true}, 204)
	items = anna.req("GET", p("/photo-albums"), nil, 200)["items"].([]any)
	if items[0].(map[string]any)["count"] != float64(0) {
		t.Fatal("archive count")
	}
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id}, "archived": false, "album_id": 0}, 204)
	anna.req("POST", p("/photo-albums"), map[string]any{"title": " "}, 422)
	anna.req("PATCH", p("/photos"), map[string]any{"ids": []string{id, id}, "archived": true}, 422)
	anna.req("GET", p("/photos?after=bad"), nil, 400)
	anna.req("GET", p("/communities/popular"), nil, 200)
}
