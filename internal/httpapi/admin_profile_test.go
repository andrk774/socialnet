package httpapi

import (
	"fmt"
	"socialnet/internal/auth"
	"testing"
)

func TestAdminProfileEditing(t *testing.T) {
	srv, admin, anna, _, _ := setupWallet(t)
	path := p(fmt.Sprintf("/admin/users/%d/profile", anna.ID))
	req := map[string]any{"first_name": "New", "last_name": "Name", "email": "changed@example.com", "birthday": "1995-04-03", "created_at": "2020-01-02T03:04:00Z"}
	anna.req("PUT", path, req, 403)
	admin.req("PUT", path, req, 204)
	card := admin.req("GET", p(fmt.Sprintf("/admin/users/%d", anna.ID)), nil, 200)
	if card["first_name"] != "New" || card["email"] != "changed@example.com" || card["birthday"] != "1995-04-03" || card["email_verified"] != false || card["active_sessions"] != float64(0) {
		t.Fatal(card)
	}
	req["email"] = "bob@example.com"
	admin.req("PUT", path, req, 409)
	req["email"] = "changed@example.com"
	req["birthday"] = "2100-01-01"
	admin.req("PUT", path, req, 422)
	password := "Admin-Changed-Password-123"
	passPath := p(fmt.Sprintf("/admin/users/%d/password", anna.ID))
	admin.req("PUT", passPath, map[string]any{"password": password}, 204)
	u, err := srv.store.UserByID(t.Context(), anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := auth.VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		t.Fatal("password not changed", err)
	}
	var leaked int
	if err := srv.store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE details::text LIKE '%'||$1||'%'`, password).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatal("password in audit", err)
	}
	admin.req("PUT", passPath, map[string]any{"password": "short"}, 422)
}
