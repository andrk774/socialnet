// Package mailer: отправка писем (подтверждение email, сброс пароля, оповещения).
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net"
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
	host, _, _ := net.SplitHostPort(s.Addr)
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Password, host)
	}
	msg := "From: " + s.From + "\r\n" +
		"To: " + m.To + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(m.Body, "\n", "\r\n")

	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(s.Addr, auth, s.From, []string{m.To}, []byte(msg)) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
