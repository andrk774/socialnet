package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// За своим Nginx (TRUST_PROXY) IP клиента берётся только из того, что
// дописал прокси: правый адрес X-Forwarded-For. Заголовки, которые клиент
// присылает сам (True-Client-IP, X-Real-IP, левые записи XFF), не должны
// менять IP — иначе подменой обходится лимит на перебор паролей.
func TestTrustProxyIgnoresSpoofedHeaders(t *testing.T) {
	_, srv, _ := setupFull(t, 3)
	srv.cfg.TrustProxy = true
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	login := func(set func(h http.Header)) int {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/login",
			bytes.NewBufferString(`{"email":"a@b.cd","password":"whatever1"}`))
		req.Header.Set("Content-Type", "application/json")
		set(req.Header)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// один и тот же клиент 203.0.113.7 (его дописал Nginx справа) с каждым
	// запросом выдумывает себе новый адрес
	limited := false
	for i := range 12 {
		fake := "198.51.100." + string(rune('1'+i%9))
		code := login(func(h http.Header) {
			h.Set("True-Client-IP", fake)
			h.Set("X-Real-IP", fake)
			h.Set("X-Forwarded-For", fake+", 203.0.113.7")
		})
		if code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("spoofed headers bypassed the rate limit")
	}

	// а другой настоящий клиент (другой правый адрес) лимит не делит
	if code := login(func(h http.Header) { h.Set("X-Forwarded-For", "203.0.113.99") }); code == http.StatusTooManyRequests {
		t.Fatalf("distinct client was limited: %d", code)
	}
}
