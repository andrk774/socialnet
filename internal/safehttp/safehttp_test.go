package safehttp

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestAllowed(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", // метаданные облака
		"100.64.0.1", "0.0.0.0", "255.255.255.255", "::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"fd00:ec2::254", "fe80::1", "64:ff9b::a00:1", "::",
	} {
		if Allowed(netip.MustParseAddr(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"87.240.132.72", "8.8.8.8", "2a00:1450:4010:c02::64"} {
		if !Allowed(netip.MustParseAddr(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
}

func TestClientRefusesLoopback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer ts.Close()
	// localhost через имя тоже не проходит: проверяется IP при соединении
	for _, u := range []string{ts.URL, strings.Replace(ts.URL, "127.0.0.1", "localhost", 1)} {
		resp, err := Client(2 * time.Second).Get(u)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("%s: connected to loopback", u)
		}
		if !strings.Contains(err.Error(), "internal address") {
			t.Fatalf("%s: unexpected error %v", u, err)
		}
	}
}
