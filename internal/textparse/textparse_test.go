package textparse

import (
	"reflect"
	"testing"
)

func TestHashtags(t *testing.T) {
	cases := map[string][]string{
		"Привет #Миасс и #миасс, #go_lang!": {"миасс", "go_lang"},
		"язык C# и цвет &#123; и #1":        nil,
		"#первый в начале":                  {"первый"},
		"почта a#b не тег, а (#тег) — тег":  {"тег"},
	}
	for in, want := range cases {
		if got := Hashtags(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Hashtags(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMentions(t *testing.T) {
	cases := map[string][]string{
		"Привет, @Anna и @anna и @bob_99!": {"anna", "bob_99"},
		"почта ivan@mail.ru не упоминание": nil,
		"@ab слишком коротко":              nil,
	}
	for in, want := range cases {
		if got := Mentions(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Mentions(%q) = %v, want %v", in, got, want)
		}
	}
}
