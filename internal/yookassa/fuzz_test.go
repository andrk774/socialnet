package yookassa

import "testing"

func FuzzParseKopecks(f *testing.F) {
	for _, s := range []string{"99.00", "0.5", "1", "-1.00", "1e9", "99999999999999999999.00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		k, err := ParseKopecks(s)
		if err != nil {
			return
		}
		if k < 0 {
			t.Fatalf("%q → negative %d", s, k)
		}
		if back, err := ParseKopecks(RUB(k).Value); err != nil || back != k {
			t.Fatalf("%q → %d → %q → %d", s, k, RUB(k).Value, back)
		}
	})
}
