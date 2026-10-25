package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func pngBytes(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func upload(t *testing.T, ts *httptest.Server, u *tuser, data []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "photo.png")
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/media", &body)
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

func TestMediaFlow(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")

	code, m := upload(t, ts, anna, pngBytes(50, 30))
	if code != 201 {
		t.Fatalf("upload: %d %v", code, m)
	}
	id := m["id"].(string)

	// отдача: правильный тип, кэш, без авторизации
	resp, err := http.Get(ts.URL + m["thumb_url"].(string))
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("serve: %v %v", resp.StatusCode, resp.Header)
	}
	resp.Body.Close()
	if r, _ := http.Get(ts.URL + "/media/" + id + "/original"); r.StatusCode != 404 {
		t.Fatal("unknown size served")
	}
	if r, _ := http.Get(ts.URL + "/media/..%2F..%2Fetc%2Fpasswd/full"); r.StatusCode != 404 {
		t.Fatal("traversal")
	}

	// неверные файлы
	if code, _ := upload(t, ts, anna, []byte("<?php echo 1; ?>")); code != 415 {
		t.Fatalf("php upload: %d", code)
	}

	// аватар: своё фото можно, чужое нельзя
	me := anna.req("PUT", p("/users/me/avatar"), map[string]any{"media_id": id}, 200)
	if me["avatar_url"] == nil {
		t.Fatalf("avatar: %v", me)
	}
	bob.req("PUT", p("/users/me/avatar"), map[string]any{"media_id": id}, 422)

	// фото в посте: своё можно, чужое нельзя
	post := anna.req("POST", p("/posts"), map[string]any{"text": "фото", "photo_ids": []string{id}}, 201)
	if photos := post["photos"].([]any); len(photos) != 1 {
		t.Fatalf("post photos: %v", post)
	}
	bob.req("POST", p("/posts"), map[string]any{"photo_ids": []string{id}}, 422)

	// пост только с фото, без текста
	anna.req("POST", p("/posts"), map[string]any{"photo_ids": []string{id}}, 201)

	// удалить чужое фото нельзя, своё можно — пропадёт из постов и аватара
	bob.req("DELETE", p("/media/%s", id), nil, 404)
	anna.req("DELETE", p("/media/%s", id), nil, 204)
	if me := anna.req("GET", p("/users/me"), nil, 200); me["avatar_url"] != nil {
		t.Fatal("avatar not cleared")
	}
	if r, _ := http.Get(ts.URL + "/media/" + id + "/full"); r.StatusCode != 404 {
		t.Fatal("deleted media still served")
	}
}
