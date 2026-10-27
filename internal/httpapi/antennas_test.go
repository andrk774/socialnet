package httpapi

import (
	"fmt"
	"testing"
)

func TestAntennas(t *testing.T) {
	ts, _, _ := setupFull(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)

	for name, bad := range map[string]map[string]any{
		"no words":  {"name": "x", "keywords": []string{}},
		"injection": {"name": "x", "keywords": []string{`таганай" or "а`}},
		"short":     {"name": "x", "keywords": []string{"т"}},
		"no name":   {"name": " ", "keywords": []string{"таганай"}},
		"too many":  {"name": "x", "keywords": []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "b1", "b2"}},
	} {
		if code, out := anna.do("POST", p("/antennas"), anna.Token, bad); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}
	a := anna.req("POST", p("/antennas"), map[string]any{"name": "Таганай и Иремель",
		"keywords": []string{"Таганай", "#иремель", "таганай"}, "excludes": []string{"продам"}}, 201)
	aid := int64(a["id"].(float64))
	if kw := a["keywords"].([]any); len(kw) != 2 || kw[1] != "иремель" {
		t.Fatalf("keywords: %v", kw)
	}

	bob.req("POST", p("/posts"), map[string]any{"text": "Были на Таганае в выходные"}, 201)       // словоформа
	bob.req("POST", p("/posts"), map[string]any{"text": "Иремель в снегу #иремель"}, 201)         // второе слово
	bob.req("POST", p("/posts"), map[string]any{"text": "Продам палатку, ходил на Таганай"}, 201) // исключение
	bob.req("POST", p("/posts"), map[string]any{"text": "Про Зюраткуль"}, 201)                    // не про то
	bob.req("POST", p("/posts"), map[string]any{"text": "Таганай только для друзей", "audience": "friends"}, 201)
	eve.req("POST", p("/posts"), map[string]any{"text": "Таганай — тайная запись", "audience": "only_me"}, 201) // не видна Анне

	list := anna.req("GET", p("/antennas"), nil, 200)["items"].([]any)
	if u := list[0].(map[string]any)["unread"].(float64); u != 3 { // Таганае, Иремель, для друзей
		t.Fatalf("unread: %v", u)
	}
	posts := items(anna.req("GET", p("/antennas/%d/posts", aid), nil, 200))
	if len(posts) != 3 { // без «продам», «Зюраткуля» и чужой «только мне»
		var texts []string
		for _, x := range posts {
			texts = append(texts, x.(map[string]any)["text"].(string))
		}
		t.Fatalf("feed: %v", texts)
	}
	if u := anna.req("GET", p("/antennas"), nil, 200)["items"].([]any)[0].(map[string]any)["unread"].(float64); u != 0 {
		t.Fatalf("unread after viewing: %v", u)
	}
	// чужую антенну не видно, не поправить
	bob.req("GET", p("/antennas/%d/posts", aid), nil, 404)
	bob.req("PUT", p("/antennas/%d", aid), map[string]any{"name": "x", "keywords": []string{"иремель"}}, 404)
	anna.req("PUT", p("/antennas/%d", aid), map[string]any{"name": "Только Иремель", "keywords": []string{"иремель"}}, 200)
	if n := len(items(anna.req("GET", p("/antennas/%d/posts", aid), nil, 200))); n != 1 {
		t.Fatalf("after edit: %d", n)
	}
	for i := 2; i <= 5; i++ {
		anna.req("POST", p("/antennas"), map[string]any{"name": fmt.Sprint(i), "keywords": []string{"урал"}}, 201)
	}
	anna.req("POST", p("/antennas"), map[string]any{"name": "6", "keywords": []string{"урал"}}, 422)
	anna.req("DELETE", p("/antennas/%d", aid), nil, 204)
}
