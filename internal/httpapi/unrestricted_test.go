package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitsDisabled(t *testing.T) {
	_, original, _ := setupFull(t, 1)
	cfg := original.cfg
	cfg.RateLimitsEnabled = false
	srv, err := New(cfg, original.store, original.log)
	if err != nil {
		t.Fatal(err)
	}
	for name, l := range map[string]limiter{
		"auth": srv.limiter("auth", 1, 1), "messages": srv.msgLimiter,
		"upload": srv.uploadLimiter, "mfa": srv.mfaLimiter,
		"mail": srv.mailLimiter, "wallet": srv.walletLimiter,
		"public": srv.publicLimiter, "apps": srv.appLimiter, "bots": srv.botLimiter,
	} {
		for i := 0; i < 100; i++ {
			if !l.allow("same-client") {
				t.Fatalf("%s blocked request %d", name, i)
			}
		}
	}
	handler := limitMiddleware(srv.limiter("auth", 1, 1))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := 0; i < 30; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("middleware blocked: %d", w.Code)
		}
	}
}
