package httpapi

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func FuzzCleanText(f *testing.F) {
	f.Add("Песня\u202e\nвторая", 200)
	f.Add("\x00\xff", 5)
	f.Fuzz(func(t *testing.T, s string, max int) {
		if max < 0 || max > 1000 {
			return
		}
		out := cleanText(s, max)
		if !utf8.ValidString(out) || utf8.RuneCountInString(out) > max {
			t.Fatalf("bad output %q", out)
		}
		for _, r := range out {
			if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
				t.Fatalf("control char %U survived", r)
			}
		}
		if out != strings.TrimSpace(out) || cleanText(out, max) != out {
			t.Fatalf("not idempotent: %q", out)
		}
	})
}
