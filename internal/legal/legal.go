// Package legal — юридические документы сайта: пользовательское соглашение,
// политика обработки персональных данных и отдельное согласие на обработку
// (152-ФЗ требует, чтобы согласие было самостоятельным документом).
//
// Тексты — шаблоны: перед запуском их должен проверить юрист, а оператор —
// подать уведомление в Роскомнадзор. Реквизиты оператора подставляются из
// настроек (LEGAL_*), в тексте — плейсхолдеры {{operator}} и т. п.
//
// Изменили документ — поднимите его Version и общий Revision: все
// пользователи при следующем входе примут новую редакцию, а до этого
// не смогут ничего менять на сайте (только читать, выйти или удалить аккаунт).
package legal

import (
	"embed"
	"html/template"
	"strings"
)

// Revision — общая редакция комплекта. Пользователь, принявший меньшую,
// должен принять документы заново.
const Revision = 1

type Doc struct {
	Key     string // terms | privacy | consent
	Title   string
	Version string // дата редакции
	file    string
}

var Docs = []Doc{
	{Key: "terms", Title: "Пользовательское соглашение", Version: "2026-10-26", file: "docs/terms.md"},
	{Key: "privacy", Title: "Политика обработки персональных данных", Version: "2026-10-26", file: "docs/privacy.md"},
	{Key: "consent", Title: "Согласие на обработку персональных данных", Version: "2026-10-26", file: "docs/consent.md"},
}

//go:embed docs/*.md
var files embed.FS

func ByKey(key string) (Doc, bool) {
	for _, d := range Docs {
		if d.Key == key {
			return d, true
		}
	}
	return Doc{}, false
}

// Operator — реквизиты оператора персональных данных.
type Operator struct {
	Name    string // ИП Иванов Иван Иванович / ООО «…»
	INN     string
	Address string
	Email   string
	SiteURL string
}

func (o Operator) replacer() *strings.Replacer {
	or := func(v, placeholder string) string {
		if strings.TrimSpace(v) == "" {
			return placeholder
		}
		return v
	}
	return strings.NewReplacer(
		"{{operator}}", or(o.Name, "[оператор: LEGAL_OPERATOR]"),
		"{{inn}}", or(o.INN, "[ИНН: LEGAL_INN]"),
		"{{address}}", or(o.Address, "[адрес: LEGAL_ADDRESS]"),
		"{{email}}", or(o.Email, "[почта: LEGAL_EMAIL]"),
		"{{site}}", or(o.SiteURL, "[адрес сайта: APP_URL]"),
	)
}

// Block — абзац, заголовок или список: всё, что умеет разметка документов.
type Block struct {
	Kind  string // h2 | p | ul
	Text  string
	Items []string
}

// Render разбирает документ: «## » — заголовок, «- » — пункт списка,
// пустая строка — конец абзаца. HTML в тексте не допускается: в шаблоне
// всё выводится как текст.
func Render(d Doc, op Operator) ([]Block, error) {
	raw, err := files.ReadFile(d.file)
	if err != nil {
		return nil, err
	}
	text := op.replacer().Replace(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	var out []Block
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, Block{Kind: "p", Text: strings.Join(para, " ")})
			para = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "## "):
			flush()
			out = append(out, Block{Kind: "h2", Text: strings.TrimPrefix(line, "## ")})
		case strings.HasPrefix(line, "- "):
			flush()
			item := strings.TrimPrefix(line, "- ")
			if n := len(out); n > 0 && out[n-1].Kind == "ul" {
				out[n-1].Items = append(out[n-1].Items, item)
			} else {
				out = append(out, Block{Kind: "ul", Items: []string{item}})
			}
		default:
			para = append(para, line)
		}
	}
	flush()
	return out, nil
}

// Page — HTML-страница документа (без скриптов, стили — общие).
var Page = template.Must(template.New("legal").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Doc.Title}} — socialnet</title>
<link rel="stylesheet" href="/static/styles.css">
</head><body class="legal-page"><main class="legal">
<p><a href="/">← socialnet</a></p>
<h1>{{.Doc.Title}}</h1>
<p class="muted">Редакция от {{.Doc.Version}}</p>
{{range .Blocks}}{{if eq .Kind "h2"}}<h2>{{.Text}}</h2>
{{else if eq .Kind "ul"}}<ul>{{range .Items}}<li>{{.}}</li>{{end}}</ul>
{{else}}<p>{{.Text}}</p>
{{end}}{{end}}
<nav class="legal-nav">{{range .All}}<a href="/legal/{{.Key}}">{{.Title}}</a> {{end}}</nav>
</main></body></html>
`))
