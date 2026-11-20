// Package mailer: отправка писем (подтверждение email, сброс пароля, оповещения).
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type Message struct {
	To      string
	Subject string
	Body    string
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// SMTP отправляет письма через SMTP-сервер. Если сервер поддерживает
// STARTTLS, net/smtp включит шифрование автоматически.
type SMTP struct {
	TLSMode  string // empty=legacy auto, starttls, tls, none
	Addr     string // host:port
	User     string
	Password string
	From     string
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	// защита от внедрения заголовков через адрес или тему
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return fmt.Errorf("invalid header value")
	}
	envelope, header, err := splitFrom(s.From)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(s.Addr)
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Password, host)
	}
	msg := "From: " + header + "\r\n" +
		"To: " + m.To + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(m.Body, "\n", "\r\n")

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dialer := &net.Dialer{}
	var conn net.Conn
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if s.TLSMode == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", s.Addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", s.Addr)
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if s.TLSMode == "starttls" || s.TLSMode == "" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(tlsConfig); err != nil {
				return err
			}
		} else if s.TLSMode == "starttls" {
			return fmt.Errorf("SMTP STARTTLS required")
		}
	}
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(envelope); err != nil {
		return err
	}
	if err = client.Rcpt(m.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = io.WriteString(writer, msg); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// splitFrom разбирает MAIL_FROM («Socialnet <no-reply@example.com>» или
// просто адрес): в SMTP-команду MAIL FROM идёт только адрес — с именем
// сервер отвечает «501 invalid FROM parameter», — а в заголовок From —
// имя и адрес, имя закодировано по RFC 2047.
func splitFrom(from string) (envelope, header string, err error) {
	if strings.ContainsAny(from, "\r\n") {
		return "", "", fmt.Errorf("invalid MAIL_FROM")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return "", "", fmt.Errorf("invalid MAIL_FROM %q: %w", from, err)
	}
	return addr.Address, addr.String(), nil
}

// Log пишет письма в лог вместо отправки — для разработки без SMTP.
type Log struct{ Log *slog.Logger }

func (l *Log) Send(_ context.Context, m Message) error {
	l.Log.Info("email (not sent: SMTP_HOST is empty)", "to", m.To, "subject", m.Subject, "body", m.Body)
	return nil
}

// Memory складывает письма в память — для тестов.
type Memory struct {
	mu   sync.Mutex
	sent []Message
}

func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *Memory) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}
