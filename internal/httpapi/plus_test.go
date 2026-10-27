package httpapi

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlusSubscription(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.cfg.PlusPrice = 199
	srv.walletLimiter = newIPRateLimiter(1000, 1000)
	admin, anna, bob := register(t, ts, "admin"), register(t, ts, "anna"), register(t, ts, "bob")
	_ = srv.store.SetRole(t.Context(), admin.ID, "admin")
	verifyEmail(t, srv, anna, bob)

	if st := anna.req("GET", p("/plus"), nil, 200); st["active"] != false || st["price"].(float64) != 199 {
		t.Fatalf("status: %v", st)
	}
	anna.req("POST", p("/plus/buy"), map[string]any{"idempotency_key": "short"}, 422)
	anna.req("POST", p("/plus/buy"), map[string]any{"idempotency_key": "buy-plus-0001"}, 402) // нет монет
	mint(t, admin, anna.ID, 500, "mint-anna-plus")

	// двойной клик: один ключ — одно списание
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, out := anna.do("POST", p("/plus/buy"), anna.Token, map[string]any{"idempotency_key": "buy-plus-0002"}); code != 201 && code != 200 {
				t.Errorf("buy: %d %v", code, out)
			}
		}()
	}
	wg.Wait()
	if b := balance(t, anna); b != 301 { // 500 − 199
		t.Fatalf("balance after one purchase: %v", b)
	}
	st := anna.req("GET", p("/plus"), nil, 200)
	exp, _ := time.Parse(time.RFC3339Nano, st["expires_at"].(string))
	if st["active"] != true || time.Until(exp) < 29*24*time.Hour {
		t.Fatalf("plus: %v", st)
	}
	// второй период продлевает от конца текущего
	anna.req("POST", p("/plus/buy"), map[string]any{"idempotency_key": "buy-plus-0003"}, 201)
	exp2, _ := time.Parse(time.RFC3339Nano, anna.req("GET", p("/plus"), nil, 200)["expires_at"].(string))
	if d := exp2.Sub(exp); d < 29*24*time.Hour {
		t.Fatalf("extension: %v", d)
	}
	checkInvariant(t, srv)

	// значок в профиле и в /users/me
	if bob.req("GET", p("/users/%d", anna.ID), nil, 200)["plus"] != true || anna.req("GET", p("/users/me"), nil, 200)["plus"] != true {
		t.Fatal("no plus badge")
	}
	if bob.req("GET", p("/users/me"), nil, 200)["plus"] != false {
		t.Fatal("bob has no plus")
	}

	// лимиты: у Боба 5 лент, у Анны — больше
	for i := range 6 {
		anna.req("POST", p("/antennas"), map[string]any{"name": "лента", "keywords": []string{"урал"}}, 201)
		code := 201
		if i == 5 {
			code = 422
		}
		bob.req("POST", p("/antennas"), map[string]any{"name": "лента", "keywords": []string{"урал"}}, code)
	}

	// клуб: список участников похода CSV — только организатору с «Плюсом»
	at := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	ah := anna.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": at}, 201)
	bh := bob.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель", "starts_at": at}, 201)
	_ = srv.store.Pool().QueryRow(t.Context(), `UPDATE users SET first_name = '=HYPERLINK("x")' WHERE id = $1 RETURNING id`, bob.ID).Scan(new(int64))
	bob.req("POST", p("/events/%d/rsvp", int64(ah["id"].(float64))), map[string]any{"status": "going"}, 200)
	bob.req("GET", p("/events/%d/participants.csv", int64(bh["id"].(float64))), nil, 402)
	bob.req("GET", p("/events/%d/participants.csv", int64(ah["id"].(float64))), nil, 403)
	req, _ := http.NewRequest("GET", ts.URL+p("/events/%d/participants.csv", int64(ah["id"].(float64))), nil)
	req.Header.Set("Authorization", "Bearer "+anna.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	csv := string(body)
	if resp.StatusCode != 200 || !strings.Contains(csv, "Имя;Фамилия;Ник;Статус") || !strings.Contains(csv, "anna;идёт") ||
		!strings.Contains(csv, `"'=HYPERLINK(""x"")"`) {
		t.Fatalf("csv: %d %q", resp.StatusCode, csv)
	}

	// автопродление: денег не хватит — выключается и приходит уведомление
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE subscriptions SET expires_at = now() + interval '10 minutes' WHERE user_id = $1`, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := srv.RunPlusRenewals(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b := balance(t, anna); b != 102 { // 500 − 2×199, на третий период не хватает
		t.Fatalf("balance after failed renewal: %v", b)
	}
	if st := anna.req("GET", p("/plus"), nil, 200); st["auto_renew"] != false {
		t.Fatalf("auto renew should be off: %v", st)
	}
	if !hasType(t, anna, "plus_renew_failed") {
		t.Error("no renewal failure notice")
	}
	// хватает — продлевается один раз, даже если продление запустить дважды
	mint(t, admin, anna.ID, 1000, "mint-anna-plus-2")
	anna.req("PATCH", p("/plus"), map[string]any{"auto_renew": true}, 200)
	for range 2 {
		if err := srv.RunPlusRenewals(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if b := balance(t, anna); b != 903 { // 102 + 1000 − 199
		t.Fatalf("renewal charged wrong: %v", b)
	}
	checkInvariant(t, srv)
}
