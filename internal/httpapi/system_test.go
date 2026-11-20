package httpapi

import (
	"errors"
	"socialnet/internal/store"
	"testing"
)

func TestSystemAndRoleAccess(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	admin, mod, user := register(t, ts, "sysadmin"), register(t, ts, "sysmod"), register(t, ts, "sysuser")
	if err := srv.store.SetRole(t.Context(), admin.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.SetRole(t.Context(), mod.ID, "moderator"); err != nil {
		t.Fatal(err)
	}
	user.req("GET", p("/admin/system"), nil, 403)
	mod.req("GET", p("/admin/system"), nil, 403)
	mod.req("PUT", p("/admin/users/%d/role", user.ID), map[string]any{"role": "admin"}, 403)
	out := admin.req("GET", p("/admin/system"), nil, 200)
	if out["database_ok"] != true || out["heap_bytes"].(float64) <= 0 || out["queues"] == nil {
		t.Fatal(out)
	}
	admin.req("PUT", p("/admin/users/%d/role", admin.ID), map[string]any{"role": "user"}, 400)
	if err := srv.store.SetRole(t.Context(), admin.ID, "user"); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("last admin: %v", err)
	}
	admin.req("PUT", p("/admin/users/%d/role", user.ID), map[string]any{"role": "admin"}, 204)
	user.req("GET", p("/admin/system"), nil, 200)
	admin.req("PUT", p("/admin/users/%d/role", user.ID), map[string]any{"role": "user"}, 204)
	user.req("GET", p("/admin/system"), nil, 403)
}
