package httpapi

import (
	"strings"
	"testing"
)

func TestAISettings(t *testing.T) {
	s, admin, user, _, _ := setupWallet(t)
	req := map[string]any{"enabled": true, "allow_users": false, "base_url": "https://api.example.com/v1", "text_model": "text-test", "image_model": "", "api_key": "private-ai-test-key"}
	user.req("PUT", p("/admin/ai"), req, 403)
	admin.req("PUT", p("/admin/ai"), req, 204)
	got := admin.req("GET", p("/admin/ai"), nil, 200)
	if got["api_key"] != nil || got["key_set"] != true {
		t.Fatal(got)
	}
	req["api_key"] = ""
	admin.req("PUT", p("/admin/ai"), req, 204)
	c, e := s.loadAI(t.Context())
	if e != nil || c.APIKey != "private-ai-test-key" {
		t.Fatal("key retention", e)
	}
	got = user.req("GET", p("/ai/status"), nil, 200)
	if got["text"] != false {
		t.Fatal(got)
	}
	user.req("POST", p("/ai/generate"), map[string]string{"kind": "text", "prompt": "hello"}, 403)
	req["enabled"] = false
	admin.req("PUT", p("/admin/ai"), req, 204)
	admin.req("POST", p("/ai/generate"), map[string]string{"kind": "text", "prompt": "hello"}, 403)
	req["base_url"] = "http://example.com"
	admin.req("PUT", p("/admin/ai"), req, 422)
	var details string
	e = s.store.Pool().QueryRow(t.Context(), `SELECT coalesce(string_agg(details::text,''),'') FROM audit_log`).Scan(&details)
	if e != nil || strings.Contains(details, "private-ai-test-key") {
		t.Fatal("key leak", e)
	}
}
