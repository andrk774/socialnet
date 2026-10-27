package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/store"
)

// handleEventICS: GET /events/{id}.ics — поход или встреча файлом iCalendar
// (RFC 5545): телефон предложит добавить в календарь, с напоминанием за
// день и за два часа. Видимость — как у самого мероприятия.
func (s *Server) handleEventICS(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Content-Disposition", contentDisposition(fmt.Sprintf("%s.ics", icsFileName(e.Title))))
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(eventICS(e, s.cfg.AppURL, time.Now())))
}

func icsFileName(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		return "событие"
	}
	if utf8.RuneCountInString(name) > 60 {
		name = string([]rune(name)[:60])
	}
	return name
}

func eventICS(e *store.Event, appURL string, now time.Time) string {
	const stamp = "20060102T150405Z"
	end := e.StartsAt.Add(3 * time.Hour)
	if e.EndsAt != nil && e.EndsAt.After(e.StartsAt) {
		end = *e.EndsAt
	}
	link := appURL + "/events/" + fmt.Sprint(e.ID)
	host := "socialnet"
	if u, err := url.Parse(appURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	var desc []string
	if e.Description != "" {
		desc = append(desc, e.Description)
	}
	if e.MeetingPoint != "" {
		desc = append(desc, "Сбор: "+e.MeetingPoint)
	}
	if e.Gear != "" {
		desc = append(desc, "Снаряжение: "+e.Gear)
	}
	if e.ControlAt != nil {
		desc = append(desc, "Контрольный срок возвращения: "+e.ControlAt.In(uralTime).Format("02.01.2006 15:04")+" (Екатеринбург)")
	}
	desc = append(desc, link)
	location := e.MeetingPoint
	if location == "" {
		location = e.Place
	}

	var b strings.Builder
	line := func(name, value string) { b.WriteString(icsFold(name + ":" + value)) }
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//socialnet//hikes//RU\r\nCALSCALE:GREGORIAN\r\nMETHOD:PUBLISH\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	line("UID", fmt.Sprintf("event-%d@%s", e.ID, host))
	line("DTSTAMP", now.UTC().Format(stamp))
	line("DTSTART", e.StartsAt.UTC().Format(stamp))
	line("DTEND", end.UTC().Format(stamp))
	line("SUMMARY", icsText(e.Title))
	line("DESCRIPTION", icsText(strings.Join(desc, "\n\n")))
	if location != "" {
		line("LOCATION", icsText(location))
	}
	line("URL", link)
	for _, before := range []string{"-P1D", "-PT2H"} {
		b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\n")
		line("DESCRIPTION", icsText(e.Title))
		line("TRIGGER", before)
		b.WriteString("END:VALARM\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

// icsText экранирует TEXT-значение (RFC 5545, 3.3.11).
func icsText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// icsFold режет строку по 75 октетов, не разрывая символы UTF-8 (3.1).
func icsFold(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		size := utf8.RuneLen(r)
		if n+size > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += size
	}
	b.WriteString("\r\n")
	return b.String()
}
