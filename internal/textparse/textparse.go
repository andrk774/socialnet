// Package textparse извлекает #хэштеги и @упоминания из текста постов.
package textparse

import (
	"regexp"
	"strings"
)

// Хэштег: буквы любого алфавита (в т.ч. кириллица), цифры, подчёркивание.
// Перед # не должно быть буквы или цифры, иначе это не тег (например, "C#").
var (
	hashtagRe = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_&])#([\p{L}\p{N}_]{1,64})`)
	mentionRe = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_@.])@([A-Za-z0-9_]{3,32})\b`)
)

const maxItems = 20

// Hashtags возвращает уникальные теги в нижнем регистре, не больше 20.
func Hashtags(text string) []string {
	return extract(hashtagRe, text, true)
}

// Mentions возвращает уникальные username без @, не больше 20.
func Mentions(text string) []string {
	return extract(mentionRe, text, true)
}

func extract(re *regexp.Regexp, text string, lower bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		v := m[1]
		key := v
		if lower {
			key = strings.ToLower(v)
		}
		// чисто цифровой хэштег (#1) тегом не считаем
		if strings.Trim(key, "0123456789") == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
		if len(out) == maxItems {
			break
		}
	}
	return out
}
