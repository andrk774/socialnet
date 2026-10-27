package httpapi

import (
	"sync"
	"testing"
	"time"
)

func TestFundraisers(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	srv.walletLimiter = newIPRateLimiter(1000, 1000)
	admin, owner, anna, bob, eve := register(t, ts, "admin"), register(t, ts, "owner"), register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	_ = srv.store.SetRole(t.Context(), admin.ID, "admin")
	verifyEmail(t, srv, anna, bob, eve)
	mint(t, admin, anna.ID, 1000, "mint-anna-f")
	mint(t, admin, bob.ID, 1000, "mint-bob-f")
	c := owner.req("POST", p("/communities"), map[string]any{"slug": "spas_club", "name": "Спасфонд клуба"}, 201)
	cid := int64(c["id"].(float64))
	dl := time.Now().Add(10 * 24 * time.Hour).UTC().Format(time.RFC3339)

	anna.req("POST", p("/communities/%d/fundraisers", cid), map[string]any{"title": "x", "goal": 100, "deadline": dl}, 403)
	for name, bad := range map[string]map[string]any{
		"goal":     {"goal": 0},
		"past":     {"deadline": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
		"far":      {"deadline": time.Now().Add(400 * 24 * time.Hour).UTC().Format(time.RFC3339)},
		"no title": {"title": " "},
	} {
		body := map[string]any{"title": "Рации для клуба", "goal": 500, "deadline": dl}
		for k, v := range bad {
			body[k] = v
		}
		if code, out := owner.do("POST", p("/communities/%d/fundraisers", cid), owner.Token, body); code != 422 {
			t.Errorf("%s accepted: %d %v", name, code, out)
		}
	}
	f := owner.req("POST", p("/communities/%d/fundraisers", cid), map[string]any{"title": "Рации для клуба", "description": "Четыре рации Baofeng", "goal": 500, "deadline": dl}, 201)
	fid := int64(f["id"].(float64))
	if f["open"] != true || f["raised"].(float64) != 0 {
		t.Fatalf("fundraiser: %v", f)
	}

	// двойной клик — одно пожертвование
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, out := anna.do("POST", p("/fundraisers/%d/donate", fid), anna.Token, map[string]any{"amount": 300, "idempotency_key": "don-anna-0001", "memo": "Удачи!"}); code != 201 && code != 200 {
				t.Errorf("donate: %d %v", code, out)
			}
		}()
	}
	wg.Wait()
	bob.req("POST", p("/fundraisers/%d/donate", fid), map[string]any{"amount": 250, "idempotency_key": "don-bob-00001", "anonymous": true}, 201)
	bob.req("POST", p("/fundraisers/%d/donate", fid), map[string]any{"amount": 5000, "idempotency_key": "don-bob-00002"}, 402)
	eve.req("POST", p("/fundraisers/%d/donate", fid), map[string]any{"amount": 10, "idempotency_key": "don-eve-00001"}, 402) // нет монет

	got := eve.req("GET", p("/fundraisers/%d", fid), nil, 200)
	fr := got["fundraiser"].(map[string]any)
	if fr["raised"].(float64) != 550 || fr["donors"].(float64) != 2 {
		t.Fatalf("progress: %v", fr)
	}
	ds := got["donations"].([]any)
	if len(ds) != 2 || ds[0].(map[string]any)["user"] != nil || ds[1].(map[string]any)["user"].(map[string]any)["username"] != "anna" {
		t.Fatalf("donations (anonymous hidden): %v", ds)
	}
	if b := balance(t, anna); b != 700 {
		t.Fatalf("anna balance: %v", b)
	}
	if w := owner.req("GET", p("/communities/%d/wallet", cid), nil, 200); w["balance"].(float64) != 550 {
		t.Fatalf("community wallet: %v", w)
	}
	if n := len(items(eve.req("GET", p("/fundraisers"), nil, 200))); n != 1 {
		t.Fatalf("open fundraisers: %d", n)
	}

	// закрыт — больше не принимает
	anna.req("POST", p("/fundraisers/%d/close", fid), nil, 403)
	owner.req("POST", p("/fundraisers/%d/close", fid), nil, 200)
	bob.req("POST", p("/fundraisers/%d/donate", fid), map[string]any{"amount": 1, "idempotency_key": "don-bob-00003"}, 409)
	// повтор старого запроса после закрытия не списывает второй раз
	if code, _ := anna.do("POST", p("/fundraisers/%d/donate", fid), anna.Token, map[string]any{"amount": 300, "idempotency_key": "don-anna-0001", "memo": "Удачи!"}); code != 200 {
		t.Fatalf("replay after close: %d", code)
	}
	if b := balance(t, anna); b != 700 {
		t.Fatalf("replay charged: %v", b)
	}
	checkInvariant(t, srv)

	// закрытое сообщество: сборы видят только участники
	cc := owner.req("POST", p("/communities"), map[string]any{"slug": "secret_club", "name": "Тайный", "visibility": "closed"}, 201)
	sf := owner.req("POST", p("/communities/%d/fundraisers", int64(cc["id"].(float64))), map[string]any{"title": "Тайный сбор", "goal": 10, "deadline": dl}, 201)
	eve.req("GET", p("/fundraisers/%d", int64(sf["id"].(float64))), nil, 404)
}
