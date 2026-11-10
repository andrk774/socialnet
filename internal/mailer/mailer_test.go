package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSplitFrom(t *testing.T) {
	for from, want := range map[string][2]string{
		"Socialnet <no-reply@localhost>": {"no-reply@localhost", `"Socialnet" <no-reply@localhost>`},
		"no-reply@example.com":           {"no-reply@example.com", "<no-reply@example.com>"},
		"Соцсеть <no-reply@example.com>": {"no-reply@example.com", "=?utf-8?q?=D0=A1=D0=BE=D1=86=D1=81=D0=B5=D1=82=D1=8C?= <no-reply@example.com>"},
	} {
		env, hdr, err := splitFrom(from)
		if err != nil || env != want[0] || hdr != want[1] {
			t.Errorf("splitFrom(%q) = %q, %q, %v; want %q, %q", from, env, hdr, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "не адрес", "a@b.c\r\nBcc: x@y.z"} {
		if _, _, err := splitFrom(bad); err == nil {
			t.Errorf("splitFrom(%q): no error", bad)
		}
	}
}

// fakeSMTP — минимальный SMTP-сервер: запоминает MAIL FROM и текст письма
// и, как настоящий, отвергает MAIL FROM с именем вместо голого адреса.
func fakeSMTP(t *testing.T) (addr string, got chan [2]string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got = make(chan [2]string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r, w := bufio.NewReader(conn), conn
		reply := func(s string) { _, _ = w.Write([]byte(s + "\r\n")) }
		reply("220 fake ESMTP")
		var from string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimRight(line, "\r\n")
			switch up := strings.ToUpper(cmd); {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				reply("250 fake")
			case strings.HasPrefix(up, "MAIL FROM:"):
				from = cmd[len("MAIL FROM:"):]
				if strings.Count(from, "<") != 1 { // имя рядом с адресом
					reply(`501 5.5.4 Syntax error in parameters or arguments (invalid FROM parameter)`)
					continue
				}
				reply("250 ok")
			case strings.HasPrefix(up, "RCPT TO:"):
				reply("250 ok")
			case up == "DATA":
				reply("354 go ahead")
				var body strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					body.WriteString(l)
				}
				got <- [2]string{from, body.String()}
				reply("250 queued")
			case up == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return ln.Addr().String(), got
}

// MAIL_FROM по умолчанию — с именем; раньше он целиком уходил в MAIL FROM,
// и сервер отвергал каждое письмо.
func TestSMTPSendWithDisplayName(t *testing.T) {
	addr, got := fakeSMTP(t)
	m := &SMTP{Addr: addr, From: "Socialnet <no-reply@localhost>"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.Send(ctx, Message{To: "mama@example.com", Subject: "Тревога", Body: "Текст"}); err != nil {
		t.Fatal(err)
	}
	r := <-got
	if r[0] != "<no-reply@localhost>" {
		t.Errorf("MAIL FROM: %q", r[0])
	}
	if !strings.Contains(r[1], "From: \"Socialnet\" <no-reply@localhost>\r\n") {
		t.Errorf("From header missing:\n%s", r[1])
	}
}
