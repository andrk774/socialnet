// Package markup — безопасная разметка статей. Текст разбирается в дерево
// блоков, а не в HTML: фронтенд рисует его компонентами, сервер — через
// html/template, поэтому ни один символ из текста не становится тегом или
// скриптом. Ссылки — только http(s), картинки — только свои фото (media:UUID).
//
// Синтаксис (подмножество Markdown):
//
//	## Заголовок            ### Подзаголовок
//	- пункт                 1. пункт
//	> цитата                ---  (разделитель)
//	![подпись](media:UUID)  — фото на всю ширину, отдельной строкой
//	**жирный**, *курсив* или _курсив_, [текст](https://…)
package markup

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxSource = 100_000 // символов
	MaxBlocks = 3000
	MaxImages = 50
)

var (
	ErrTooLong  = errors.New("article is too long")
	ErrTooMany  = errors.New("too many blocks")
	ErrTooManyI = errors.New("too many images")
)

type Span struct {
	T    string `json:"t"`              // text | b | i | a
	Text string `json:"text"`           // текст (для a — подпись)
	Href string `json:"href,omitempty"` // только для a: http(s)
}

type Block struct {
	Kind    string   `json:"kind"` // h2 | h3 | p | ul | ol | quote | hr | img
	Spans   []Span   `json:"spans,omitempty"`
	Items   [][]Span `json:"items,omitempty"`
	MediaID string   `json:"media_id,omitempty"`
	Caption string   `json:"caption,omitempty"`
}

var (
	imgRe  = regexp.MustCompile(`^!\[([^\]]{0,300})\]\(media:([0-9a-fA-F-]{36})\)$`)
	olRe   = regexp.MustCompile(`^\d{1,3}[.)] `)
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Parse разбирает текст статьи. Управляющие символы (кроме перевода строки
// и табуляции) и символы смены направления удаляются.
func Parse(src string) ([]Block, error) {
	if utf8.RuneCountInString(src) > MaxSource {
		return nil, ErrTooLong
	}
	src = clean(src)
	var out []Block
	var para []string
	images := 0
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: "p", Spans: Inline(strings.Join(para, " "))})
			para = nil
		}
	}
	add := func(b Block) {
		flush()
		if b.Kind == "ul" || b.Kind == "ol" || b.Kind == "quote" {
			if n := len(out); n > 0 && out[n-1].Kind == b.Kind {
				if b.Kind == "quote" {
					out[n-1].Spans = append(out[n-1].Spans, Span{T: "text", Text: " "})
					out[n-1].Spans = append(out[n-1].Spans, b.Spans...)
				} else {
					out[n-1].Items = append(out[n-1].Items, b.Items...)
				}
				return
			}
		}
		out = append(out, b)
	}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			flush()
		case strings.HasPrefix(t, "### "):
			add(Block{Kind: "h3", Spans: []Span{{T: "text", Text: strings.TrimSpace(t[4:])}}})
		case strings.HasPrefix(t, "## "):
			add(Block{Kind: "h2", Spans: []Span{{T: "text", Text: strings.TrimSpace(t[3:])}}})
		case t == "---" || t == "***":
			add(Block{Kind: "hr"})
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			add(Block{Kind: "ul", Items: [][]Span{Inline(strings.TrimSpace(t[2:]))}})
		case olRe.MatchString(t):
			add(Block{Kind: "ol", Items: [][]Span{Inline(strings.TrimSpace(olRe.ReplaceAllString(t, "")))}})
		case strings.HasPrefix(t, ">"):
			add(Block{Kind: "quote", Spans: Inline(strings.TrimSpace(t[1:]))})
		default:
			if m := imgRe.FindStringSubmatch(t); m != nil && uuidRe.MatchString(strings.ToLower(m[2])) {
				images++
				add(Block{Kind: "img", MediaID: strings.ToLower(m[2]), Caption: strings.TrimSpace(m[1])})
				continue
			}
			para = append(para, t)
		}
		if len(out) > MaxBlocks {
			return nil, ErrTooMany
		}
	}
	flush()
	if len(out) > MaxBlocks {
		return nil, ErrTooMany
	}
	if images > MaxImages {
		return nil, ErrTooManyI
	}
	return out, nil
}

// MediaIDs — фото, на которые ссылается статья.
func MediaIDs(blocks []Block) []string {
	var ids []string
	seen := map[string]bool{}
	for _, b := range blocks {
		if b.Kind == "img" && !seen[b.MediaID] {
			seen[b.MediaID] = true
			ids = append(ids, b.MediaID)
		}
	}
	return ids
}

// PlainText — для описания страницы и поиска.
func PlainText(blocks []Block, max int) string {
	var b strings.Builder
	for _, bl := range blocks {
		spans := bl.Spans
		for _, it := range bl.Items {
			spans = append(spans, it...)
		}
		for _, s := range spans {
			b.WriteString(s.Text)
		}
		b.WriteString(" ")
		if b.Len() > max*4 {
			break
		}
	}
	r := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return string(r)
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return -1
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200e, r == 0x200f:
			return -1 // смена направления текста — приём для подделки ссылок
		case r == utf8.RuneError:
			return -1
		}
		return r
	}, s)
}

// SafeURL пропускает только абсолютные http(s)-адреса без учётных данных.
func SafeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2000 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// Inline разбирает **жирный**, *курсив*/_курсив_ и [ссылки](https://…).
// Вложенность не поддерживается: внутри выделения — простой текст.
func Inline(s string) []Span {
	var out []Span
	var text strings.Builder
	emit := func(t, txt, href string) {
		if text.Len() > 0 {
			out = append(out, Span{T: "text", Text: text.String()})
			text.Reset()
		}
		if txt != "" {
			out = append(out, Span{T: t, Text: txt, Href: href})
		}
	}
	for i := 0; i < len(s); {
		rest := s[i:]
		if strings.HasPrefix(rest, "**") {
			if j := strings.Index(rest[2:], "**"); j > 0 {
				emit("b", rest[2:2+j], "")
				i += 4 + j
				continue
			}
		}
		if c := rest[0]; c == '*' || c == '_' {
			if j := strings.IndexByte(rest[1:], c); j > 0 && !strings.Contains(rest[1:1+j], "\n") &&
				(i == 0 || !isWord(s[i-1])) && (2+j >= len(rest) || !isWord(rest[1+j+1])) {
				emit("i", rest[1:1+j], "")
				i += 2 + j
				continue
			}
		}
		if rest[0] == '[' {
			if j := strings.Index(rest, "]("); j > 0 {
				if k := strings.IndexByte(rest[j+2:], ')'); k > 0 {
					if href, ok := SafeURL(rest[j+2 : j+2+k]); ok {
						emit("a", rest[1:j], href)
						i += j + 3 + k
						continue
					}
				}
			}
		}
		_, size := utf8.DecodeRuneInString(rest)
		text.WriteString(rest[:size])
		i += size
	}
	emit("", "", "")
	return out
}

func isWord(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b >= 0x80
}
