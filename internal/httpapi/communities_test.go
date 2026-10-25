package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCommunitiesRBAC(t *testing.T) {
	ts := setup(t, 1000)
	owner, adm, ed, mem, out := register(t, ts, "owner"), register(t, ts, "adm"), register(t, ts, "editor"), register(t, ts, "mem"), register(t, ts, "outsider")

	c := owner.req("POST", p("/communities"), map[string]any{"slug": "miass_news", "name": "Новости Миасса"}, 201)
	cid := int64(c["id"].(float64))
	if c["my_role"] != "owner" {
		t.Fatalf("owner role: %v", c)
	}
	adm.req("POST", p("/communities"), map[string]any{"slug": "MIASS_NEWS", "name": "дубль"}, 409)

	for _, u := range []*tuser{adm, ed, mem} {
		u.req("POST", p("/communities/%d/join", cid), nil, 204)
	}
	owner.req("PUT", p("/communities/%d/members/%d/role", cid, adm.ID), map[string]any{"role": "admin"}, 200)
	adm.req("PUT", p("/communities/%d/members/%d/role", cid, ed.ID), map[string]any{"role": "editor"}, 200)

	// эскалация прав запрещена
	adm.req("PUT", p("/communities/%d/members/%d/role", cid, mem.ID), map[string]any{"role": "admin"}, 403) // админ не назначает админов
	ed.req("PUT", p("/communities/%d/members/%d/role", cid, mem.ID), map[string]any{"role": "editor"}, 403) // редактор не управляет
	mem.req("PUT", p("/communities/%d/members/%d/role", cid, mem.ID), map[string]any{"role": "admin"}, 403) // сам себя
	adm.req("PUT", p("/communities/%d/members/%d/role", cid, owner.ID), map[string]any{"role": "member"}, 403)
	adm.req("DELETE", p("/communities/%d/members/%d", cid, owner.ID), nil, 403)
	adm.req("PUT", p("/communities/%d/members/%d/role", cid, mem.ID), map[string]any{"role": "owner"}, 422)
	out.req("PUT", p("/communities/%d/members/%d/role", cid, mem.ID), map[string]any{"role": "editor"}, 403)
	adm.req("PUT", p("/communities/%d/members/%d/role", cid, out.ID), map[string]any{"role": "editor"}, 404) // не участник

	// публиковать могут редакторы и выше
	post := ed.req("POST", p("/posts"), map[string]any{"text": "Новость дня", "community_id": cid}, 201)
	pid := int64(post["id"].(float64))
	if post["community"].(map[string]any)["slug"] != "miass_news" {
		t.Fatalf("community post: %v", post)
	}
	mem.req("POST", p("/posts"), map[string]any{"text": "спам", "community_id": cid}, 403)
	out.req("POST", p("/posts"), map[string]any{"text": "спам", "community_id": cid}, 403)

	// пост сообщества видят все и он есть в ленте участников
	if n := len(items(out.req("GET", p("/communities/%d/posts", cid), nil, 200))); n != 1 {
		t.Fatalf("wall: %d", n)
	}
	if n := len(items(mem.req("GET", p("/feed"), nil, 200))); n != 1 {
		t.Fatalf("member feed: %d", n)
	}
	if n := len(items(out.req("GET", p("/feed"), nil, 200))); n != 0 {
		t.Fatalf("outsider feed: %d", n)
	}

	// удалить пост сообщества может админ, но не обычный участник
	mem.req("DELETE", p("/posts/%d", pid), nil, 403)
	adm.req("DELETE", p("/posts/%d", pid), nil, 204)

	// редактирование сообщества
	ed.req("PATCH", p("/communities/%d", cid), map[string]any{"name": "Захват"}, 403)
	adm.req("PATCH", p("/communities/%d", cid), map[string]any{"description": "Городские новости"}, 200)

	// исключение и выход
	adm.req("DELETE", p("/communities/%d/members/%d", cid, mem.ID), nil, 204)
	owner.req("POST", p("/communities/%d/leave", cid), nil, 409)
	ed.req("POST", p("/communities/%d/leave", cid), nil, 204)
	if n := len(items(owner.req("GET", p("/communities/%d/members", cid), nil, 200))); n != 2 {
		t.Fatalf("members: %d", n)
	}
	owner.req("GET", p("/communities/by-slug/Miass_News"), nil, 200)
	if n := len(items(adm.req("GET", p("/communities"), nil, 200))); n != 1 {
		t.Fatalf("my communities: %d", n)
	}
}

func TestSearch(t *testing.T) {
	ts := setup(t, 1000)
	anna := register(t, ts, "anna_petrova")
	register(t, ts, "anna_smirnova")
	hidden := register(t, ts, "anna_hidden")
	troll := register(t, ts, "anna_troll")
	hidden.req("PATCH", p("/users/me/privacy"), map[string]any{"searchable": false}, 200)
	troll.req("POST", p("/users/%d/block", anna.ID), nil, 204)

	res := items(anna.req("GET", p("/search?q=anna"), nil, 200))
	names := map[string]bool{}
	for _, u := range res {
		names[u.(map[string]any)["username"].(string)] = true
	}
	if !names["anna_smirnova"] || names["anna_hidden"] || names["anna_troll"] {
		t.Fatalf("search results: %v", names)
	}
	// % и _ — обычные символы, а не шаблон «всё подряд»
	if n := len(items(anna.req("GET", p("/search?q=%s", "%25%25"), nil, 200))); n != 0 {
		t.Fatalf("LIKE wildcard injection: %d results", n)
	}
	anna.req("GET", p("/search?q=a"), nil, 400)

	anna.req("POST", p("/communities"), map[string]any{"slug": "go_miass", "name": "Go-разработчики Миасса"}, 201)
	if n := len(items(anna.req("GET", p("/search?type=communities&q=миасс"), nil, 200))); n != 1 {
		t.Fatalf("community search: %d", n)
	}
}

func TestModeration(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	admin, mod, spammer, victim := register(t, ts, "admin"), register(t, ts, "mod"), register(t, ts, "spammer"), register(t, ts, "victim")
	if err := srv.store.SetRole(t.Context(), admin.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	admin.req("PUT", p("/admin/users/%d/role", mod.ID), map[string]any{"role": "moderator"}, 204)
	mod.req("PUT", p("/admin/users/%d/role", victim.ID), map[string]any{"role": "admin"}, 403) // модератор не админ

	post := spammer.req("POST", p("/posts"), map[string]any{"text": "купи слона"}, 201)
	pid := int64(post["id"].(float64))

	// жалоба: одна от человека, на существующий объект
	rep := victim.req("POST", p("/reports"), map[string]any{"target_type": "post", "target_id": pid, "reason": "спам"}, 201)
	victim.req("POST", p("/reports"), map[string]any{"target_type": "post", "target_id": pid, "reason": "спам"}, 409)
	victim.req("POST", p("/reports"), map[string]any{"target_type": "post", "target_id": 999999, "reason": "?"}, 404)
	victim.req("POST", p("/reports"), map[string]any{"target_type": "users; DROP TABLE", "target_id": 1, "reason": "?"}, 422)
	admin.req("POST", p("/reports"), map[string]any{"target_type": "post", "target_id": pid, "reason": "реклама"}, 201)

	// обычный пользователь не видит панель модератора
	victim.req("GET", p("/mod/reports"), nil, 403)
	if n := len(items(mod.req("GET", p("/mod/reports"), nil, 200))); n != 2 {
		t.Fatalf("open reports: %d", n)
	}

	// скрыть пост: пропадает для всех, жалобы закрываются
	rid := int64(rep["id"].(float64))
	mod.req("POST", p("/mod/reports/%d/resolve", rid), map[string]any{"action": "hide", "note": "спам"}, 204)
	mod.req("POST", p("/mod/reports/%d/resolve", rid), map[string]any{"action": "hide"}, 409)
	victim.req("GET", p("/posts/%d", pid), nil, 404)
	if n := len(items(mod.req("GET", p("/mod/reports"), nil, 200))); n != 0 {
		t.Fatalf("reports not closed: %d", n)
	}

	// бан: сессии гаснут сразу, вход невозможен, профиль пропадает
	mod.req("POST", p("/mod/users/%d/ban", spammer.ID), map[string]any{"reason": "спам"}, 204)
	if code, out := spammer.do("GET", "/api/v1/feed", spammer.Token, nil); code != 403 || out["error"].(map[string]any)["code"] != "banned" {
		t.Fatalf("banned user still has access: %d %v", code, out)
	}
	if _, _, code := login(t, ts, "spammer@example.com", "password-spammer", ""); code != 403 {
		t.Fatal("banned user can log in")
	}
	victim.req("GET", p("/users/%d", spammer.ID), nil, 404)

	// иерархия: модератор не банит модераторов и админов
	mod.req("POST", p("/mod/users/%d/ban", admin.ID), map[string]any{"reason": "бунт"}, 403)
	mod.req("POST", p("/mod/users/%d/ban", mod.ID), map[string]any{"reason": "сам"}, 403)

	mod.req("POST", p("/mod/users/%d/unban", spammer.ID), nil, 204)
	if _, _, code := login(t, ts, "spammer@example.com", "password-spammer", ""); code != 200 {
		t.Fatal("unbanned user cannot log in")
	}
	mod.req("POST", p("/mod/posts/%d/unhide", pid), nil, 204)
	victim.req("GET", p("/posts/%d", pid), nil, 200)
	mod.req("POST", p("/mod/users/%d/hide", pid), nil, 404) // неизвестный тип

	// журнал модерации
	if n := len(items(admin.req("GET", p("/mod/log"), nil, 200))); n < 4 {
		t.Fatalf("mod log: %d", n)
	}
}

func TestSecurityTxtAndVulnReports(t *testing.T) {
	ts, srv, box := setupFull(t, 1000)
	srv.cfg.SecurityContact = "security@app.test"

	resp, err := http.Get(ts.URL + "/.well-known/security.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, field := range []string{"Contact: mailto:security@app.test", "Expires: ", "Policy: "} {
		if !strings.Contains(string(body), field) {
			t.Fatalf("security.txt missing %q:\n%s", field, body)
		}
	}

	anon := newClient(t, ts.URL)
	code, out := anon.do("POST", "/api/v1/security/vulnerability-reports", "", map[string]any{
		"contact": "researcher@mail.ru", "title": "IDOR в /api/v1/x", "description": "шаги воспроизведения...",
	})
	if code != 202 || out["id"] == nil {
		t.Fatalf("vuln report: %d %v", code, out)
	}
	m := waitMail(t, box, "security@app.test", "уязвимости")
	if strings.Contains(m.Body, "IDOR") {
		t.Fatal("untrusted report text copied into email")
	}

	admin := register(t, ts, "admin")
	user := register(t, ts, "user")
	_ = srv.store.SetRole(t.Context(), admin.ID, "admin")
	user.req("GET", p("/admin/vulnerability-reports"), nil, 403)
	list := items(admin.req("GET", p("/admin/vulnerability-reports"), nil, 200))
	if len(list) != 1 {
		t.Fatalf("vuln list: %v", list)
	}
	vid := int64(list[0].(map[string]any)["id"].(float64))
	admin.req("PATCH", p("/admin/vulnerability-reports/%d", vid), map[string]any{"status": "triaged"}, 204)

	// не больше 5 отчётов в час с одного IP
	var limited bool
	for i := 0; i < 10; i++ {
		if c, _ := anon.do("POST", "/api/v1/security/vulnerability-reports", "", map[string]any{
			"contact": "x@y.z", "title": "t", "description": "d"}); c == 429 {
			limited = true
		}
	}
	if !limited {
		t.Fatal("vulnerability form not rate limited")
	}
}
