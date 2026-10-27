package markup

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParse(t *testing.T) {
	src := "## Двуглавая сопка\n\nВышли в **7 утра** от _Белого ключа_.\nПодъём [по тропе](https://example.com/t?a=1) занял час.\n\n" +
		"- вода\n- палатка\n\n1. старт\n2) финиш\n\n> Тише едешь —\n> дальше будешь\n\n---\n" +
		"![Вид с вершины](media:0b7d1c5e-2f3a-4c6b-9d8e-1a2b3c4d5e6f)\n" +
		"<script>alert(1)</script> [плохая](javascript:alert(1)) [ещё](//evil.example) snake_case_name 2*3*4"
	bl, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, b := range bl {
		kinds = append(kinds, b.Kind)
	}
	if got := strings.Join(kinds, ","); got != "h2,p,ul,ol,quote,hr,img,p" {
		t.Fatalf("kinds: %s", got)
	}
	p := bl[1].Spans
	if p[1] != (Span{T: "b", Text: "7 утра"}) || p[3] != (Span{T: "i", Text: "Белого ключа"}) ||
		p[5] != (Span{T: "a", Text: "по тропе", Href: "https://example.com/t?a=1"}) {
		t.Fatalf("inline: %+v", p)
	}
	if len(bl[2].Items) != 2 || len(bl[3].Items) != 2 || bl[6].MediaID != "0b7d1c5e-2f3a-4c6b-9d8e-1a2b3c4d5e6f" || bl[6].Caption != "Вид с вершины" {
		t.Fatalf("blocks: %+v", bl)
	}
	last := bl[7].Spans
	if len(last) != 1 || last[0].T != "text" || !strings.Contains(last[0].Text, "<script>alert(1)</script>") ||
		!strings.Contains(last[0].Text, "snake_case_name 2*3*4") {
		t.Errorf("unsafe links must stay plain text: %+v", last)
	}
	if j, _ := json.Marshal(last); strings.Contains(string(j), `"href"`) {
		t.Errorf("href kept: %s", j)
	}
	if ids := MediaIDs(bl); len(ids) != 1 {
		t.Fatalf("media: %v", ids)
	}
	if pt := PlainText(bl, 30); utf8.RuneCountInString(pt) != 30 || !strings.HasPrefix(pt, "Двуглавая сопка Вышли в 7") {
		t.Fatalf("plain: %q", pt)
	}
}

func TestParseLimits(t *testing.T) {
	if _, err := Parse(strings.Repeat("я", MaxSource+1)); !errors.Is(err, ErrTooLong) {
		t.Fatalf("too long: %v", err)
	}
	if _, err := Parse(strings.Repeat("---\n", MaxBlocks+5)); !errors.Is(err, ErrTooMany) {
		t.Fatalf("too many: %v", err)
	}
	bl, _ := Parse("при\u202eвет\x00\x07!")
	if bl[0].Spans[0].Text != "привет!" {
		t.Fatalf("controls: %q", bl[0].Spans[0].Text)
	}
	for _, u := range []string{"https://user:pw@example.com", "ftp://x", "data:text/html,1", "https://", "/local"} {
		if _, ok := SafeURL(u); ok {
			t.Errorf("unsafe url accepted: %s", u)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("## a\n- **b** _c_ [d](https://e)\n> q\n![x](media:0b7d1c5e-2f3a-4c6b-9d8e-1a2b3c4d5e6f)")
	f.Add("[[(**_*]](https://a)**")
	f.Fuzz(func(t *testing.T, s string) {
		bl, err := Parse(s)
		if err != nil {
			return
		}
		for _, b := range bl {
			spans := b.Spans
			for _, it := range b.Items {
				spans = append(spans, it...)
			}
			for _, sp := range spans {
				if sp.T == "a" {
					if _, ok := SafeURL(sp.Href); !ok {
						t.Fatalf("unsafe href %q", sp.Href)
					}
				} else if sp.Href != "" {
					t.Fatalf("href on %s", sp.T)
				}
				if !utf8.ValidString(sp.Text) {
					t.Fatalf("invalid utf8")
				}
			}
		}
	})
}
