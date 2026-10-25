package httpapi

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"socialnet/internal/store"
	"socialnet/internal/webpush"
)

func TestPresence(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	befriend(t, anna, bob)
	bob.req("PATCH", p("/users/me/privacy"), map[string]any{"online_visibility": "friends"}, 200)

	// друг видит точный статус
	pr := anna.req("GET", p("/users/%d", bob.ID), nil, 200)["presence"].(map[string]any)
	if pr["online"] != true || pr["last_seen"] == nil {
		t.Fatalf("friend presence: %v", pr)
	}
	// чужой — только «был недавно»
	pr = eve.req("GET", p("/users/%d", bob.ID), nil, 200)["presence"].(map[string]any)
	if pr["online"] != nil || pr["last_seen"] != nil || pr["last_seen_approx"] != "recently" {
		t.Fatalf("stranger presence leaks: %v", pr)
	}
	// заблокированный — ничего
	bob.req("POST", p("/users/%d/block", eve.ID), nil, 204)
	pr = eve.req("GET", p("/users/%d", bob.ID), nil, 200)["presence"].(map[string]any)
	if len(pr) != 1 { // только user_id
		t.Fatalf("blocked user sees presence: %v", pr)
	}

	list := anna.req("POST", p("/presence"), map[string]any{"ids": []int64{bob.ID, eve.ID}}, 200)["items"].([]any)
	if len(list) != 2 {
		t.Fatalf("batch presence: %v", list)
	}
	anna.req("POST", p("/presence"), map[string]any{"ids": []int64{}}, 400)
}

func TestNotificationSettings(t *testing.T) {
	ts := setup(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	post := anna.req("POST", p("/posts"), map[string]any{"text": "пост"}, 201)
	pid := int64(post["id"].(float64))

	got := anna.req("GET", p("/users/me/notification-settings"), nil, 200)
	if got["settings"].(map[string]any)["push_enabled"] != true {
		t.Fatalf("defaults: %v", got)
	}

	anna.req("PUT", p("/users/me/notification-settings"), map[string]any{"muted_types": []string{"hack"}}, 422)
	anna.req("PUT", p("/users/me/notification-settings"), map[string]any{"timezone": "Mars/Olympus"}, 422)
	anna.req("PUT", p("/users/me/notification-settings"), map[string]any{"quiet_start": 1380}, 422)
	anna.req("PUT", p("/users/me/notification-settings"), map[string]any{
		"muted_types": []string{"like", "like"}, "email_digest": false, "push_enabled": true,
		"quiet_start": 1380, "quiet_end": 480, "timezone": "Asia/Yekaterinburg",
	}, 200)

	// лайк заглушён — уведомления нет, комментарий — есть
	bob.req("POST", p("/posts/%d/like", pid), nil, 200)
	bob.req("POST", p("/posts/%d/comments", pid), map[string]any{"text": "класс"}, 201)
	notes := items(anna.req("GET", p("/notifications"), nil, 200))
	if len(notes) != 1 || notes[0].(map[string]any)["type"] != "comment" {
		t.Fatalf("muted like still notified: %v", notes)
	}
}

func TestQuietHours(t *testing.T) {
	ptr := func(v int) *int { return &v }
	night := store.NotificationSettings{QuietStart: ptr(23 * 60), QuietEnd: ptr(8 * 60), Timezone: "Asia/Yekaterinburg"}
	yekt := time.FixedZone("YEKT", 5*3600)
	cases := map[string]bool{"23:30": true, "02:00": true, "07:59": true, "08:00": false, "12:00": false, "22:59": false}
	for hm, want := range cases {
		tt, _ := time.ParseInLocation("15:04", hm, yekt)
		now := time.Date(2026, 10, 25, tt.Hour(), tt.Minute(), 0, 0, yekt)
		if got := inQuietHours(night, now); got != want {
			t.Errorf("%s: got %v want %v", hm, got, want)
		}
	}
	if inQuietHours(store.DefaultNotificationSettings(), time.Now()) {
		t.Error("no quiet hours configured")
	}
}

// fakePushService — подставной push-сервис: принимает зашифрованные сообщения.
type fakePushService struct {
	mu     sync.Mutex
	bodies [][]byte
	auth   []string
	status int
}

func (f *fakePushService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, b)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	st := f.status
	f.mu.Unlock()
	if st == 0 {
		st = http.StatusCreated
	}
	w.WriteHeader(st)
}

func (f *fakePushService) wait(t *testing.T, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.bodies) >= n {
			out := append([][]byte(nil), f.bodies...)
			f.mu.Unlock()
			return out
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d push messages", n)
	return nil
}

func (f *fakePushService) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func TestWebPush(t *testing.T) {
	ts, srv, _ := setupFull(t, 1000)
	pub, priv, _ := webpush.GenerateVAPID()
	srv.vapid, _ = webpush.NewVAPID(pub, priv, "mailto:security@app.test")
	fake := &fakePushService{}
	pushSrv := httptest.NewTLSServer(fake)
	defer pushSrv.Close()
	srv.pushClient = pushSrv.Client()
	srv.pushHosts = []string{"127.0.0.1"}

	anna, bob, eve := register(t, ts, "anna"), register(t, ts, "bob"), register(t, ts, "eve")
	if k := anna.req("GET", p("/push/vapid-public-key"), nil, 200); k["public_key"] != pub {
		t.Fatal("vapid key")
	}

	// «браузер» Анны создаёт ключи подписки
	browser, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSecret := make([]byte, 16)
	rand.Read(authSecret)
	sub := map[string]any{
		"endpoint": pushSrv.URL + "/push/anna-device",
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(browser.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(authSecret),
		},
	}
	anna.req("POST", p("/push/subscriptions"), sub, 204)

	// SSRF и чужие подписки
	evil := map[string]any{"endpoint": "https://169.254.169.254/latest", "keys": sub["keys"]}
	anna.req("POST", p("/push/subscriptions"), evil, 422)
	badKey := map[string]any{"endpoint": sub["endpoint"], "keys": map[string]string{"p256dh": "AAAA", "auth": "AAAA"}}
	anna.req("POST", p("/push/subscriptions"), badKey, 422)
	eve.req("POST", p("/push/subscriptions"), sub, 409)

	// Анна не в приложении → лайк приходит push-ом
	post := anna.req("POST", p("/posts"), map[string]any{"text": "пост"}, 201)
	bob.req("POST", p("/posts/%d/like", int64(post["id"].(float64))), nil, 200)
	bodies := fake.wait(t, 1)

	plain, err := webpush.Decrypt(bodies[0], browser, authSecret)
	if err != nil {
		t.Fatalf("browser cannot decrypt push: %v", err)
	}
	var msg map[string]string
	_ = json.Unmarshal(plain, &msg)
	if msg["type"] != "like" || !strings.Contains(msg["body"], "bob") {
		t.Fatalf("push payload: %s", plain)
	}
	if strings.Contains(string(bodies[0]), "like") {
		t.Fatal("push payload not encrypted")
	}
	if !strings.HasPrefix(fake.auth[0], "vapid t=") || !strings.Contains(fake.auth[0], "k="+pub) {
		t.Fatalf("vapid header: %s", fake.auth[0])
	}

	// Анна в приложении → push не нужен, событие идёт по WebSocket
	conn := wsConnect(t, ts, anna)
	eve.req("POST", p("/users/%d/friend", anna.ID), nil, 200)
	waitEvent(t, conn, "notification")
	conn.CloseNow()
	time.Sleep(100 * time.Millisecond)
	if fake.count() != 1 {
		t.Fatalf("push sent while user online: %d", fake.count())
	}

	// подписка протухла (410) → удаляется
	fake.mu.Lock()
	fake.status = http.StatusGone
	fake.mu.Unlock()
	bob.req("POST", p("/users/%d/messages", anna.ID), map[string]any{"body": "привет"}, 201)
	fake.wait(t, 2)
	deadline := time.Now().Add(2 * time.Second)
	for {
		subs, _ := srv.store.PushSubs(t.Context(), anna.ID)
		if len(subs) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired subscription not removed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEmailDigest(t *testing.T) {
	ts, srv, box := setupFull(t, 1000)
	anna, bob := register(t, ts, "anna"), register(t, ts, "bob")
	token := tokenInMail.FindStringSubmatch(waitMail(t, box, "anna@example.com", "Подтвердите").Body)[1]
	anna.req("POST", p("/auth/email/verify"), map[string]any{"token": token}, 204)
	bob.req("POST", p("/users/%d/friend", anna.ID), nil, 200)

	ctx := t.Context()
	if n, _ := srv.RunDigest(ctx); n != 0 {
		t.Fatal("digest sent to an active user")
	}
	// Анна давно не заходила, уведомление висит больше часа
	pool := srv.store.Pool()
	pool.Exec(ctx, `UPDATE users SET last_seen_at = now() - interval '3 hours' WHERE id = $1`, anna.ID)
	pool.Exec(ctx, `UPDATE notifications SET created_at = now() - interval '2 hours' WHERE user_id = $1`, anna.ID)

	if n, err := srv.RunDigest(ctx); n != 1 || err != nil {
		t.Fatalf("digest: %d %v", n, err)
	}
	waitMail(t, box, "anna@example.com", "непрочитанных")
	if n, _ := srv.RunDigest(ctx); n != 0 {
		t.Fatal("digest sent twice within 24h")
	}
	// у Боба email не подтверждён — писем нет
	pool.Exec(ctx, `UPDATE users SET last_seen_at = now() - interval '3 hours' WHERE id = $1`, bob.ID)
	if n, _ := srv.RunDigest(ctx); n != 0 {
		t.Fatal("digest to unverified email")
	}
}
