package textparse

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// go test -fuzz=FuzzExtract ./internal/textparse
func FuzzExtract(f *testing.F) {
	for _, s := range []string{"#Миасс @anna", "C# a#b &#1;", "@@@###", "#" + strings.Repeat("я", 70), "\xff#\xfe"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, list := range [][]string{Hashtags(s), Mentions(s)} {
			if len(list) > maxItems {
				t.Fatalf("too many items: %d", len(list))
			}
			seen := map[string]bool{}
			for _, v := range list {
				if v == "" || seen[v] || v != strings.ToLower(v) || !utf8.ValidString(v) || utf8.RuneCountInString(v) > 64 {
					t.Fatalf("bad item %q in %q", v, s)
				}
				seen[v] = true
			}
		}
	})
}
