package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"socialnet/internal/yookassa"
)

// fakeYooKassa — имитация API ЮKassa v3 с идемпотентностью по Idempotence-Key.
type fakeYooKassa struct {
	mu       sync.Mutex
	payments map[string]*yookassa.Payment
	byKey    map[string]string
	requests []yookassa.CreateRequest
	keys     []string
	down     bool
	n        int
}

func (f *fakeYooKassa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "shop" || p != "secret" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"type":"error","code":"invalid_credentials","description":"bad auth"}`))
		return
	}
	if f.down {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"type":"error","code":"internal_server_error"}`))
		return
	}
	switch {
	case r.Method == "POST" && r.URL.Path == "/payments":
		key := r.Header.Get("Idempotence-Key")
		var req yookassa.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.requests, f.keys = append(f.requests, req), append(f.keys, key)
		if id, ok := f.byKey[key]; ok {
			_ = json.NewEncoder(w).Encode(f.payments[id])
			return
		}
		f.n++
		id := fmt.Sprintf("2f5d%04d-000f-5000-8000-1c4b8a2e0001", f.n)
		p := &yookassa.Payment{ID: id, Status: "pending", Amount: req.Amount, Metadata: req.Metadata, Test: true,
			Confirmation: &yookassa.Confirmation{Type: "redirect", ConfirmationURL: "https://yoomoney.ru/checkout/payments/v2/contract?orderId=" + id}}
		f.payments[id], f.byKey[key] = p, id
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/payments/"):
		p, ok := f.payments[strings.TrimPrefix(r.URL.Path, "/payments/")]
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"type":"error","code":"not_found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(p)
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeYooKassa) set(id, status string, mutate ...func(*yookassa.Payment)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.payments[id]
	p.Status, p.Paid = status, status == "succeeded"
	for _, m := range mutate {
		m(p)
	}
}

func webhook(t *testing.T, ts *httptest.Server, event, id string) int {
	t.Helper()
	body := fmt.Sprintf(`{"type":"notification","event":%q,"object":{"id":%q,"status":"succeeded","paid":true,"amount":{"value":"999999.00","currency":"RUB"}}}`, event, id)
	resp, err := http.Post(ts.URL+"/api/v1/payments/yookassa", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestBuyCoinsYooKassa(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	balance := func(u *tuser) float64 { return u.req("GET", p("/wallet"), nil, 200)["balance"].(float64) }

	// без настроек ЮKassa покупка выключена
	if out := anna.req("GET", p("/wallet/packages"), nil, 200); out["enabled"] != false || len(items(out)) != 3 {
		t.Fatalf("packages: %v", out)
	}
	anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 503)

	fake := &fakeYooKassa{payments: map[string]*yookassa.Payment{}, byKey: map[string]string{}}
	mock := httptest.NewServer(fake)
	defer mock.Close()
	srv.yookassa = yookassa.New(mock.URL, "shop", "secret")
	srv.cfg.YooKassaVATCode = 11
	srv.cfg.YooKassaCheckIP = true
	srv.walletLimiter = newIPRateLimiter(100, 100)

	anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 403) // email не подтверждён
	srv.store.Pool().Exec(t.Context(), `UPDATE users SET email_verified_at = now()`)         //nolint:errcheck
	anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_1"}, 422)

	pay := anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 201)
	pid := pay["id"].(string)
	if pay["status"] != "pending" || !strings.HasPrefix(pay["confirmation_url"].(string), "https://yoomoney.ru/") {
		t.Fatalf("purchase: %v", pay)
	}
	req := fake.requests[0]
	it := req.Receipt.Items[0]
	if fake.keys[0] != pid || req.Amount.Value != "99.00" || !req.Capture || req.Metadata["payment_id"] != pid ||
		req.Receipt.Customer.Email != "anna@example.com" || it.VATCode != 11 || it.PaymentSubject != "service" ||
		it.PaymentMode != "full_payment" || req.Confirmation.ReturnURL != "https://app.test/wallet?payment="+pid {
		t.Fatalf("create request: key=%s %+v", fake.keys[0], req)
	}
	remoteID := fake.byKey[pid]

	// уведомление не с адреса ЮKassa — отказ
	if code := webhook(t, ts, "payment.succeeded", remoteID); code != 403 {
		t.Fatalf("untrusted ip: %d", code)
	}
	srv.cfg.YooKassaCheckIP = false // httptest приходит с 127.0.0.1

	// поддельное уведомление «оплачено», а ЮKassa говорит pending — монет нет
	if code := webhook(t, ts, "payment.succeeded", remoteID); code != 200 || balance(anna) != 0 {
		t.Fatalf("forged notification credited coins: %d", code)
	}
	fake.set(remoteID, "succeeded")
	for i := 0; i < 3; i++ { // ЮKassa может прислать уведомление несколько раз
		if code := webhook(t, ts, "payment.succeeded", remoteID); code != 200 {
			t.Fatalf("webhook: %d", code)
		}
	}
	if b := balance(anna); b != 100 {
		t.Fatalf("balance after purchase: %v", b)
	}
	if got := anna.req("GET", p("/wallet/purchases/%s", pid), nil, 200); got["status"] != "succeeded" {
		t.Fatalf("purchase status: %v", got)
	}
	bob.req("GET", p("/wallet/purchases/%s", pid), nil, 404)
	if webhook(t, ts, "payment.succeeded", "unknown-id") != 200 {
		t.Fatal("unknown payment must be acknowledged")
	}

	// гонка: уведомление и возврат пользователя одновременно — начисление одно
	pay2 := anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_550"}, 201)
	fake.set(fake.byKey[pay2["id"].(string)], "succeeded")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); webhook(t, ts, "payment.succeeded", fake.byKey[pay2["id"].(string)]) }()
		go func() {
			defer wg.Done()
			anna.do("GET", p("/wallet/purchases/%s", pay2["id"]), anna.Token, nil)
		}()
	}
	wg.Wait()
	if b := balance(anna); b != 650 {
		t.Fatalf("balance after concurrent settle: %v", b)
	}

	// у ЮKassa другая сумма — не начисляем, просим повторить, пишем в лог
	pay3 := anna.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 201)
	fake.set(fake.byKey[pay3["id"].(string)], "succeeded", func(p *yookassa.Payment) { p.Amount.Value = "1.00" })
	srv.log = slog.New(slog.DiscardHandler)
	if code := webhook(t, ts, "payment.succeeded", fake.byKey[pay3["id"].(string)]); code != 502 || balance(anna) != 650 {
		t.Fatalf("amount mismatch: %d", code)
	}
	srv.log = slog.New(&failOnError{t: t})

	// отмена
	pay4 := bob.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 201)
	fake.set(fake.byKey[pay4["id"].(string)], "canceled")
	if got := bob.req("GET", p("/wallet/purchases/%s", pay4["id"]), nil, 200); got["status"] != "canceled" || balance(bob) != 0 {
		t.Fatalf("canceled: %v", got)
	}

	// шлюз недоступен
	fake.down = true
	srv.log = slog.New(slog.DiscardHandler)
	bob.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 502)
	fake.down = false
	if list := items(bob.req("GET", p("/wallet/purchases"), nil, 200)); list[0].(map[string]any)["status"] != "error" {
		t.Fatalf("failed purchase: %v", list[0])
	}
	// не больше 5 незавершённых покупок в час
	for i := 0; i < 5; i++ {
		bob.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 201)
	}
	bob.req("POST", p("/wallet/purchases"), map[string]any{"package_id": "coins_100"}, 429)

	// монеты с покупки — в истории кошелька как purchase
	hist := items(anna.req("GET", p("/wallet"), nil, 200)["transactions"].(map[string]any))
	if len(hist) != 2 || hist[0].(map[string]any)["kind"] != "purchase" {
		t.Fatalf("history: %v", hist)
	}
}
