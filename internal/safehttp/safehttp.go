// Package safehttp — HTTP-клиент для запросов на адреса, которые задают
// пользователи (Web Push, Callback API). Защищает от SSRF: не подключается
// к внутренним, служебным и зарезервированным адресам. Проверка идёт в момент
// соединения, по уже разрешённому IP, поэтому DNS rebinding тоже не проходит.
package safehttp

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// Сети, куда ходить нельзя, помимо loopback/private/link-local.
var blocked = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",       // «этот» хост
		"100.64.0.0/10",   // CGNAT (часто — внутренние сети облаков)
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // бенчмарки
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // зарезервировано, включая broadcast
		"64:ff9b::/96",    // NAT64: через него можно добраться до IPv4 внутри
		"64:ff9b:1::/48",
		"2001:db8::/32", // документация
		"fec0::/10",     // устаревшие site-local
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// Allowed — можно ли подключаться к адресу.
func Allowed(ip netip.Addr) bool {
	ip = ip.Unmap() // ::ffff:127.0.0.1 — это 127.0.0.1
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !Allowed(ip) {
		return fmt.Errorf("refusing to connect to internal address %s", host)
	}
	return nil
}

// Client: без прокси из окружения (он бы обошёл проверку адреса), без
// редиректов (могут увести во внутреннюю сеть), с общим таймаутом.
func Client(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: control}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:            dialer.DialContext,
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  timeout,
			MaxResponseHeaderBytes: 16 << 10,
			MaxIdleConnsPerHost:    2,
			IdleConnTimeout:        30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
