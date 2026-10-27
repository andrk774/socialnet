package httpapi

import (
	"strings"
	"testing"
	"time"

	"socialnet/internal/mailer"
)

func mailsTo(t *testing.T, m *mailer.Memory, to string, n int) []mailer.Message {
	t.Helper()
	for range 100 {
		var got []mailer.Message
		for _, msg := range m.Sent() {
			if msg.To == to {
				got = append(got, msg)
			}
		}
		if len(got) >= n {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no mail for %s", to)
	return nil
}

// contactMail — письма не о подтверждении почты (то есть доверенным контактам).
func contactMail(m *mailer.Memory) int {
	n := 0
	for _, msg := range m.Sent() {
		if msg.Subject != "Подтвердите email" {
			n++
		}
	}
	return n
}

func TestHikeControlDeadline(t *testing.T) {
	ts, srv, mail := setupFull(t, 1000)
	org, bob, eve := register(t, ts, "org"), register(t, ts, "bob"), register(t, ts, "eve")
	verifyEmail(t, srv, org, bob)
	start := time.Now().Add(48 * time.Hour).UTC()
	iso := func(t time.Time) string { return t.Format(time.RFC3339) }

	org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": iso(start), "control_at": iso(start.Add(-time.Hour))}, 422)
	org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Таганай", "starts_at": iso(start), "control_at": iso(start.Add(40 * 24 * time.Hour))}, 422)
	org.req("POST", p("/events"), map[string]any{"title": "Встреча", "starts_at": iso(start), "control_at": iso(start.Add(time.Hour))}, 422)

	h := org.req("POST", p("/events"), map[string]any{
		"kind": "hike", "title": "Откликной гребень", "starts_at": iso(start), "control_at": iso(start.Add(10 * time.Hour)),
		"meeting_point": "Ж/д вокзал Златоуста",
	}, 201)
	hid := int64(h["id"].(float64))
	bob.req("POST", p("/events/%d/rsvp", hid), map[string]any{"status": "going"}, 200)

	// контакты: только идущий участник с подтверждённой почтой и согласием контакта
	contact := map[string]any{"name": "Мама", "email": "Mama@Example.com", "consent": true}
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Мама", "email": "mama@example.com"}, 422)
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Мама", "email": "не почта", "consent": true}, 422)
	eve.req("POST", p("/events/%d/contacts", hid), contact, 403) // почта не подтверждена
	verifyEmail(t, srv, eve)
	eve.req("POST", p("/events/%d/contacts", hid), contact, 403) // не участник
	c := bob.req("POST", p("/events/%d/contacts", hid), contact, 201)
	if c["email"] != "mama@example.com" {
		t.Fatalf("contact: %v", c)
	}
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Брат", "email": "brat@example.com", "consent": true}, 201)
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Друг", "email": "drug@example.com", "consent": true}, 201)
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Ещё", "email": "esche@example.com", "consent": true}, 422)
	org.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Жена", "email": "zhena@example.com", "consent": true}, 201)
	// чужие контакты не видны даже организатору
	if n := len(items(org.req("GET", p("/events/%d/contacts", hid), nil, 200))); n != 1 {
		t.Fatalf("organizer sees others' contacts: %d", n)
	}
	bob.req("DELETE", p("/events/%d/contacts/%d", hid, int64(c["id"].(float64))), nil, 204)
	org.req("DELETE", p("/events/%d/contacts/%d", hid, int64(c["id"].(float64))), nil, 404)
	bob.req("POST", p("/events/%d/contacts", hid), contact, 201)

	// до срока ничего не уходит
	if err := srv.RunControl(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := contactMail(mail); n != 0 {
		t.Fatalf("mail before deadline: %d", n)
	}

	// за час до срока — напоминание участникам
	setControl := func(at time.Time) {
		if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE events SET starts_at = $2, control_at = $3 WHERE id = $1`,
			hid, at.Add(-10*time.Hour), at); err != nil {
			t.Fatal(err)
		}
	}
	setControl(time.Now().Add(30 * time.Minute))
	if err := srv.RunControl(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !has(notifTypes(t, bob, hid), "hike_control_soon") {
		t.Error("no reminder before deadline")
	}

	// срок прошёл — письма контактам, один раз
	setControl(time.Now().Add(-time.Minute))
	for range 2 {
		if err := srv.RunControl(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	msgs := mailsTo(t, mail, "mama@example.com", 1)
	time.Sleep(100 * time.Millisecond)
	if len(mailsTo(t, mail, "mama@example.com", 1)) != 1 {
		t.Fatal("alert sent twice")
	}
	body := msgs[0].Body
	for _, want := range []string{"Откликной гребень", "112", "Ж/д вокзал Златоуста", "bob", "UTC+5", "/events/"} {
		if !strings.Contains(body, want) {
			t.Errorf("alert body lacks %q:\n%s", want, body)
		}
	}
	mailsTo(t, mail, "zhena@example.com", 1)
	mailsTo(t, mail, "brat@example.com", 1)
	if got := bob.req("GET", p("/events/%d", hid), nil, 200); got["alert_sent"] != true {
		t.Fatalf("alert_sent: %v", got)
	}
	// после тревоги срок не двигается, новые контакты не добавляются
	org.req("PATCH", p("/events/%d", hid), map[string]any{"control_at": iso(time.Now().Add(5 * time.Hour))}, 422)
	bob.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Сосед", "email": "sosed@example.com", "consent": true}, 409)

	// вернулись: отбой тем, кому писали, контакты стёрты
	eve.req("POST", p("/events/%d/returned", hid), nil, 403)
	bob.req("POST", p("/events/%d/returned", hid), nil, 204)
	bob.req("POST", p("/events/%d/returned", hid), nil, 409)
	if all := mailsTo(t, mail, "mama@example.com", 2); !strings.Contains(all[1].Subject, "Отбой") {
		t.Fatalf("all-clear: %v", all[1])
	}
	var left int
	_ = srv.store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM hike_contacts`).Scan(&left)
	if left != 0 {
		t.Fatalf("contacts kept after return: %d", left)
	}
	if !has(notifTypes(t, org, hid), "hike_returned") {
		t.Error("organizer not notified about the return")
	}
}

func TestHikeContactsCleanup(t *testing.T) {
	ts, srv, mail := setupFull(t, 1000)
	org := register(t, ts, "org")
	verifyEmail(t, srv, org)
	start := time.Now().Add(48 * time.Hour).UTC()
	h := org.req("POST", p("/events"), map[string]any{"kind": "hike", "title": "Иремель",
		"starts_at": start.Format(time.RFC3339), "control_at": start.Add(8 * time.Hour).Format(time.RFC3339)}, 201)
	hid := int64(h["id"].(float64))
	org.req("POST", p("/events/%d/contacts", hid), map[string]any{"name": "Жена", "email": "zhena@example.com", "consent": true}, 201)
	// вернулись до срока — писем нет, контакты удалены
	org.req("POST", p("/events/%d/returned", hid), nil, 204)
	if _, err := srv.store.Pool().Exec(t.Context(), `UPDATE events SET starts_at = now() - interval '2 days', control_at = now() - interval '1 day' WHERE id = $1`, hid); err != nil {
		t.Fatal(err)
	}
	if err := srv.RunControl(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if n := contactMail(mail); n != 0 {
		t.Fatalf("mail after return: %d", n)
	}
	var left int
	_ = srv.store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM hike_contacts`).Scan(&left)
	if left != 0 {
		t.Fatalf("contacts left: %d", left)
	}
}
