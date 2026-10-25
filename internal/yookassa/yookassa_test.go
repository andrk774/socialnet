package yookassa

import (
	"net/netip"
	"testing"
)

func TestAmounts(t *testing.T) {
	for k, v := range map[int64]string{9900: "99.00", 5: "0.05", 100050: "1000.50", 0: "0.00"} {
		if got := RUB(k).Value; got != v {
			t.Errorf("RUB(%d) = %s", k, got)
		}
		if got, err := ParseKopecks(v); err != nil || got != k {
			t.Errorf("ParseKopecks(%s) = %d %v", v, got, err)
		}
	}
	for _, bad := range []string{"", "1.234", "-1.00", "1e3", "x.00", "1.-5"} {
		if _, err := ParseKopecks(bad); err == nil {
			t.Errorf("ParseKopecks(%q) accepted", bad)
		}
	}
	if v, _ := ParseKopecks("10.5"); v != 1050 {
		t.Errorf("10.5 → %d", v)
	}
}

func TestNotificationIPs(t *testing.T) {
	for _, s := range []string{"185.71.76.5", "185.71.77.31", "77.75.156.11", "77.75.154.200", "2a02:5180::1", "::ffff:185.71.76.1"} {
		if !FromYooKassa(netip.MustParseAddr(s)) {
			t.Errorf("%s should be trusted", s)
		}
	}
	for _, s := range []string{"185.71.76.32", "77.75.156.12", "8.8.8.8", "127.0.0.1"} {
		if FromYooKassa(netip.MustParseAddr(s)) {
			t.Errorf("%s should not be trusted", s)
		}
	}
}
