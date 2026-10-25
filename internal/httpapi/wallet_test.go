package httpapi

import (
	"fmt"
	"sync"
	"testing"
)

func verifyEmail(t *testing.T, srv *Server, users ...*tuser) {
	t.Helper()
	for _, u := range users {
		if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE users SET email_verified_at = now() WHERE id = $1`, u.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func mint(t *testing.T, admin *tuser, userID int64, amount int, key string) {
	t.Helper()
	admin.req("POST", p("/admin/wallet/mint"), map[string]any{"user_id": userID, "amount": amount, "idempotency_key": key}, 201)
}

func balance(t *testing.T, u *tuser) float64 {
	t.Helper()
	return u.req("GET", p("/wallet"), nil, 200)["balance"].(float64)
}

func checkInvariant(t *testing.T, srv *Server) {
	t.Helper()
	if total, err := srv.store.TotalBalance(t.Context()); err != nil || total != 0 {
		t.Fatalf("ledger invariant broken: sum of balances = %d (%v)", total, err)
	}
}

func setupWallet(t *testing.T) (*Server, *tuser, *tuser, *tuser, func(string) *tuser) {
	ts, srv, _ := setupFull(t, 1000)
	srv.walletLimiter = newIPRateLimiter(1000, 1000)
	admin, anna, bob := register(t, ts, "admin"), register(t, ts, "anna"), register(t, ts, "bob")
	_ = srv.store.SetRole(t.Context(), admin.ID, "admin")
	verifyEmail(t, srv, anna, bob)
	return srv, admin, anna, bob, func(name string) *tuser { return register(t, ts, name) }
}

func TestWallet(t *testing.T) {
	srv, admin, anna, bob, reg := setupWallet(t)
	eve := reg("eve") // email не подтверждён

	eve.req("POST", p("/wallet/daily-bonus"), nil, 403)
	anna.req("POST", p("/wallet/daily-bonus"), nil, 201)
	anna.req("POST", p("/wallet/daily-bonus"), nil, 409)
	if balance(t, anna) != 5 {
		t.Fatal("bonus not credited")
	}

	tr := func(u *tuser, to int64, amount int, key string, want int) map[string]any {
		return u.req("POST", p("/wallet/transfer"), map[string]any{"to_user_id": to, "amount": amount, "idempotency_key": key, "memo": "за пиццу"}, want)
	}
	tr(anna, bob.ID, 100, "k1", 402) // не хватает
	mint(t, admin, anna.ID, 1000, "mint-1")
	admin.req("POST", p("/admin/wallet/mint"), map[string]any{"user_id": anna.ID, "amount": 1000, "idempotency_key": "mint-1"}, 200) // повтор
	if balance(t, anna) != 1005 {
		t.Fatalf("mint replay credited twice: %v", balance(t, anna))
	}

	out := tr(anna, bob.ID, 100, "k2", 201)
	if out["balance"].(float64) != 905 || balance(t, bob) != 100 {
		t.Fatalf("transfer: %v", out)
	}
	// повтор того же запроса (например, после обрыва связи) — без второго списания
	if out := tr(anna, bob.ID, 100, "k2", 200); out["replayed"] != true || balance(t, anna) != 905 {
		t.Fatalf("idempotent replay charged twice: %v", out)
	}
	tr(anna, bob.ID, 50, "k2", 422)                                                                 // тот же ключ, другая сумма
	tr(anna, anna.ID, 10, "k3", 400)                                                                // себе
	tr(anna, bob.ID, -5, "k4", 422)                                                                 // отрицательная сумма
	tr(anna, bob.ID, 2_000_000, "k5", 422)                                                          // больше лимита
	anna.req("POST", p("/wallet/transfer"), map[string]any{"to_user_id": bob.ID, "amount": 1}, 422) // без ключа
	bob.req("POST", p("/users/%d/block", anna.ID), nil, 204)
	tr(anna, bob.ID, 10, "k6", 403)

	// история с направлением и контрагентом
	hist := items(anna.req("GET", p("/wallet"), nil, 200)["transactions"].(map[string]any))
	last := hist[0].(map[string]any)
	if last["direction"] != "out" || last["amount"].(float64) != 100 || last["counterparty"].(map[string]any)["id"].(float64) != float64(bob.ID) {
		t.Fatalf("history: %v", last)
	}
	// получатель узнал о переводе
	found := false
	for _, n := range items(bob.req("GET", p("/notifications"), nil, 200)) {
		found = found || n.(map[string]any)["type"] == "transfer"
	}
	if !found {
		t.Fatal("no transfer notification")
	}
	anna.req("POST", p("/admin/wallet/mint"), map[string]any{"user_id": anna.ID, "amount": 1, "idempotency_key": "x"}, 403)
	checkInvariant(t, srv)
}

func TestWalletConcurrency(t *testing.T) {
	srv, admin, anna, bob, _ := setupWallet(t)
	mint(t, admin, anna.ID, 100, "m1")
	mint(t, admin, bob.ID, 100, "m2")

	// 20 параллельных переводов по 10 со счёта, где 100: пройдут ровно 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := anna.do("POST", "/api/v1/wallet/transfer", anna.Token,
				map[string]any{"to_user_id": bob.ID, "amount": 10, "idempotency_key": fmt.Sprintf("race-%d", i)})
			mu.Lock()
			codes[code]++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if codes[201] != 10 || codes[402] != 10 || balance(t, anna) != 0 || balance(t, bob) != 200 {
		t.Fatalf("double spend: codes=%v anna=%v bob=%v", codes, balance(t, anna), balance(t, bob))
	}

	// один и тот же ключ 10 раз одновременно — одно списание
	mint(t, admin, anna.ID, 100, "m3")
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			anna.do("POST", "/api/v1/wallet/transfer", anna.Token,
				map[string]any{"to_user_id": bob.ID, "amount": 30, "idempotency_key": "same-key"})
		}()
	}
	wg.Wait()
	if balance(t, anna) != 70 {
		t.Fatalf("same idempotency key charged %v times", (100-balance(t, anna))/30)
	}

	// встречные переводы A↔B не должны вызывать deadlock
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			anna.do("POST", "/api/v1/wallet/transfer", anna.Token,
				map[string]any{"to_user_id": bob.ID, "amount": 1, "idempotency_key": fmt.Sprintf("ab-%d", i)})
		}(i)
		go func(i int) {
			defer wg.Done()
			bob.do("POST", "/api/v1/wallet/transfer", bob.Token,
				map[string]any{"to_user_id": anna.ID, "amount": 1, "idempotency_key": fmt.Sprintf("ba-%d", i)})
		}(i)
	}
	wg.Wait()
	if balance(t, anna)+balance(t, bob) != 300 {
		t.Fatal("coins lost or created in cross transfers")
	}
	checkInvariant(t, srv)
}

func TestGifts(t *testing.T) {
	srv, admin, anna, bob, _ := setupWallet(t)
	cat := anna.req("GET", p("/gifts/catalog"), nil, 200)["items"].([]any)
	heart := cat[0].(map[string]any) // самый дешёвый — 10
	hid := int(heart["id"].(float64))

	send := func(key string, anon bool, want int) map[string]any {
		return anna.req("POST", p("/users/%d/gifts", bob.ID), map[string]any{
			"gift_id": hid, "message": "С днём рождения!", "anonymous": anon, "idempotency_key": key}, want)
	}
	send("g1", false, 402)
	mint(t, admin, anna.ID, 25, "m1")
	send("g1", false, 201)
	send("g1", false, 201) // повтор: тот же подарок, без второго списания
	send("g2", true, 201)
	if balance(t, anna) != 5 {
		t.Fatalf("balance after gifts: %v", balance(t, anna))
	}
	anna.req("POST", p("/users/%d/gifts", bob.ID), map[string]any{"gift_id": 999, "idempotency_key": "g3"}, 404)

	gifts := items(bob.req("GET", p("/users/%d/gifts", bob.ID), nil, 200))
	if len(gifts) != 2 {
		t.Fatalf("gifts: %v", gifts)
	}
	if gifts[0].(map[string]any)["sender"] != nil {
		t.Fatal("anonymous gift reveals sender")
	}
	if gifts[1].(map[string]any)["sender"].(map[string]any)["username"] != "anna" {
		t.Fatal("named gift lost sender")
	}
	// в уведомлении об анонимном подарке нет отправителя
	notes := items(bob.req("GET", p("/notifications"), nil, 200))
	if n := notes[0].(map[string]any); n["type"] != "gift" || n["actor"] != nil {
		t.Fatalf("anonymous gift notification: %v", n)
	}
	checkInvariant(t, srv)
}

func TestCommunityWallet(t *testing.T) {
	srv, admin, anna, bob, reg := setupWallet(t)
	carl := reg("carl")
	verifyEmail(t, srv, carl)
	c := anna.req("POST", p("/communities"), map[string]any{"slug": "fund", "name": "Фонд"}, 201)
	cid := int64(c["id"].(float64))
	bob.req("POST", p("/communities/%d/join", cid), nil, 204)
	anna.req("PUT", p("/communities/%d/members/%d/role", cid, bob.ID), map[string]any{"role": "admin"}, 200)

	mint(t, admin, carl.ID, 100, "m1")
	carl.req("POST", p("/communities/%d/donate", cid), map[string]any{"amount": 60, "idempotency_key": "d1"}, 201)

	carl.req("GET", p("/communities/%d/wallet", cid), nil, 403)
	if w := bob.req("GET", p("/communities/%d/wallet", cid), nil, 200); w["balance"].(float64) != 60 {
		t.Fatalf("community wallet: %v", w)
	}
	pay := func(u *tuser, to int64, amount int, key string, want int) {
		u.req("POST", p("/communities/%d/wallet/payout", cid), map[string]any{"to_user_id": to, "amount": amount, "idempotency_key": key}, want)
	}
	pay(bob, bob.ID, 10, "p1", 403)   // админ не владелец
	pay(anna, carl.ID, 10, "p2", 404) // не участник
	pay(anna, bob.ID, 100, "p3", 402) // больше, чем есть
	pay(anna, bob.ID, 40, "p4", 201)
	if balance(t, bob) != 40 {
		t.Fatal("payout not received")
	}
	checkInvariant(t, srv)
}
