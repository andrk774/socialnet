package httpapi

import (
	"strings"
	"testing"
)

func TestSiteSettings(t *testing.T) {
	s, admin, user, _, _ := setupWallet(t)
	req := map[string]any{"public_url": "https://social.example.com/", "ingress_class": "nginx", "tls_secret": "socialnet-tls"}
	user.req("PUT", p("/admin/site"), req, 403)
	admin.req("PUT", p("/admin/site"), req, 204)
	if s.publicURL(t.Context()) != "https://social.example.com" {
		t.Fatal("URL not applied")
	}
	got := admin.req("GET", p("/admin/site"), nil, 200)
	if got["public_url"] != "https://social.example.com" {
		t.Fatal(got)
	}
	qr := user.req("POST", p("/auth/qr/start"), nil, 201)
	if !strings.HasPrefix(qr["url"].(string), "https://social.example.com/qr-login?") {
		t.Fatal(qr["url"])
	}
	for _, bad := range []string{"javascript:alert(1)", "https://user:pass@example.com", "https://example.com/path", "https://example.com?x=1"} {
		req["public_url"] = bad
		admin.req("PUT", p("/admin/site"), req, 422)
	}
}
