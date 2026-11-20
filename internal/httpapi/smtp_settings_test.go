package httpapi

import "testing"

func TestSMTPSettings(t *testing.T) {
	s, admin, user, _, _ := setupWallet(t)
	body := map[string]any{"host": "smtp.example.com", "port": 587, "username": "test@example.com", "sender": "Socialnet <test@example.com>", "tls_mode": "starttls", "password": "private-smtp-password"}
	user.req("GET", p("/admin/smtp"), nil, 403)
	user.req("PUT", p("/admin/smtp"), body, 403)
	admin.req("PUT", p("/admin/smtp"), body, 204)
	cfg := admin.req("GET", p("/admin/smtp"), nil, 200)
	if cfg["password"] != nil || cfg["password_set"] != true {
		t.Fatal(cfg)
	}
	body["password"] = ""
	admin.req("PUT", p("/admin/smtp"), body, 204)
	c, e := s.loadSMTP(t.Context())
	if e != nil || c.Password != "private-smtp-password" {
		t.Fatal("password retention", e)
	}
	body["tls_mode"] = "none"
	admin.req("PUT", p("/admin/smtp"), body, 422)
	body["username"] = ""
	body["clear_password"] = true
	admin.req("PUT", p("/admin/smtp"), body, 204)
	c, e = s.loadSMTP(t.Context())
	if e != nil || c.Password != "" {
		t.Fatal("password deletion", e)
	}
	body["sender"] = "bad\r\nBcc: victim@example.com"
	admin.req("PUT", p("/admin/smtp"), body, 422)
	var leaks int
	e = s.store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE details::text LIKE '%private-smtp-password%'`).Scan(&leaks)
	if e != nil || leaks != 0 {
		t.Fatal("password leaked", e)
	}
}
