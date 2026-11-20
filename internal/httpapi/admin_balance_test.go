package httpapi

import (
	"fmt"
	"testing"
)

func TestAdminBalanceAdjustment(t *testing.T) {
	srv, admin, anna, bob, _ := setupWallet(t)
	path := p(fmt.Sprintf("/admin/users/%d/balance", anna.ID))
	req := map[string]any{"amount": 100, "direction": "add", "memo": "Correction", "idempotency_key": "adjust-1"}
	anna.req("POST", path, req, 403)
	_ = srv.store.SetRole(t.Context(), bob.ID, "moderator")
	bob.req("POST", path, req, 403)
	admin.req("POST", path, req, 201)
	admin.req("POST", path, req, 200)
	if balance(t, anna) != 100 {
		t.Fatal("credit/replay")
	}
	admin.req("POST", p(fmt.Sprintf("/admin/users/%d/balance", bob.ID)), req, 422)
	req["memo"] = "Different"
	admin.req("POST", path, req, 422)
	req["memo"] = "Correction"
	req["direction"] = "remove"
	admin.req("POST", path, req, 422)
	req["idempotency_key"] = "adjust-2"
	req["amount"] = 40
	admin.req("POST", path, req, 201)
	admin.req("POST", path, req, 200)
	if balance(t, anna) != 60 {
		t.Fatal("debit/replay")
	}
	req["idempotency_key"] = "adjust-3"
	req["amount"] = 61
	admin.req("POST", path, req, 402)
	req["amount"] = -1
	admin.req("POST", path, req, 422)
	req["amount"] = 1
	req["memo"] = " "
	admin.req("POST", path, req, 422)
	req["memo"] = "Correction"
	req["direction"] = "invalid"
	admin.req("POST", path, req, 422)
	checkInvariant(t, srv)
	cardPath := p(fmt.Sprintf("/admin/users/%d", anna.ID))
	card := admin.req("GET", cardPath, nil, 200)
	if card["subscription"].(map[string]any)["active"] != false {
		t.Fatal("unexpected subscription")
	}
	if _, err := srv.store.Pool().Exec(t.Context(), `INSERT INTO subscriptions(user_id,expires_at,auto_renew) VALUES ($1,now()+interval '30 days',true)`, anna.ID); err != nil {
		t.Fatal(err)
	}
	card = admin.req("GET", cardPath, nil, 200)
	sub := card["subscription"].(map[string]any)
	if sub["active"] != true || sub["auto_renew"] != true || sub["expires_at"] == nil {
		t.Fatal("subscription missing")
	}
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE subscriptions SET expires_at=now()-interval '1 day' WHERE user_id=$1`, anna.ID); err != nil {
		t.Fatal(err)
	}
	card = admin.req("GET", cardPath, nil, 200)
	if card["subscription"].(map[string]any)["active"] != false {
		t.Fatal("expired subscription active")
	}
	history := anna.req("GET", p("/wallet"), nil, 200)
	if len(history["transactions"].(map[string]any)["items"].([]any)) != 2 {
		t.Fatal("missing ledger history")
	}
}
