package httpapi

import "testing"

func TestTURNSettings(t *testing.T) {
	s, admin, user, _, _ := setupWallet(t)
	t.Setenv("CALLS_TURN_URL", "")
	t.Setenv("CALLS_TURN_SECRET", "")
	secret := "test-turn-secret-123456789"
	body := map[string]any{"enabled": true, "turn_url": "turn:turn.example.org:3478?transport=udp", "stun_url": "stun:turn.example.org:3478", "secret": secret}
	user.req("PUT", p("/admin/turn"), body, 403)
	user.req("GET", p("/admin/turn"), nil, 403)
	admin.req("PUT", p("/admin/turn"), body, 204)
	c := admin.req("GET", p("/admin/turn"), nil, 200)
	if c["secret"] != nil || c["secret_set"] != true {
		t.Fatal(c)
	}
	body["secret"] = ""
	admin.req("PUT", p("/admin/turn"), body, 204)
	cfg, err := s.loadTURN(t.Context())
	if err != nil || cfg.Secret != secret {
		t.Fatal("secret not retained", err)
	}
	ice := user.req("GET", p("/calls/config"), nil, 200)
	if ice["relay_configured"] != true {
		t.Fatal(ice)
	}
	body["enabled"] = false
	admin.req("PUT", p("/admin/turn"), body, 204)
	ice = user.req("GET", p("/calls/config"), nil, 200)
	if ice["relay_configured"] != false {
		t.Fatal(ice)
	}
	body["turn_url"] = "https://invalid.example.org"
	admin.req("PUT", p("/admin/turn"), body, 422)
	var leaks int
	err = s.store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE details::text LIKE '%'||$1||'%'`, secret).Scan(&leaks)
	if err != nil || leaks != 0 {
		t.Fatal("audit secret leak", err)
	}
}

func TestICEValidation(t *testing.T) {
	for _, v := range []string{"https://host", "turn://host", "turn:host?secret=x", "turn:user@host", "turn:host/path"} {
		if validICE(v, true) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"turn:host:3478?transport=udp", "turns:host:5349?transport=tcp", "turn:[::1]:3478"} {
		if !validICE(v, true) {
			t.Fatal(v)
		}
	}
}
