package httpapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"net/mail"
	"socialnet/internal/mailer"
	"strconv"
	"strings"
)

type smtpSettings struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Sender        string `json:"sender"`
	TLS           string `json:"tls_mode"`
	Password      string `json:"password,omitempty"`
	PasswordSet   bool   `json:"password_set"`
	ClearPassword bool   `json:"clear_password,omitempty"`
	Source        string `json:"source"`
}

func (s *Server) loadSMTP(ctx context.Context) (smtpSettings, error) {
	c := smtpSettings{}
	var enc []byte
	err := s.store.Pool().QueryRow(ctx, `SELECT host,port,username,sender,tls_mode,password_enc FROM smtp_settings WHERE id=true`).Scan(&c.Host, &c.Port, &c.Username, &c.Sender, &c.TLS, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Host = s.cfg.SMTPHost
		c.Port, _ = strconv.Atoi(s.cfg.SMTPPort)
		c.Username = s.cfg.SMTPUser
		c.Sender = s.cfg.MailFrom
		c.Password = s.cfg.SMTPPassword
		c.TLS = "auto"
		c.Source = "environment"
	} else if err != nil {
		return c, err
	} else {
		c.Source = "database"
		p, e := s.box.Open(enc)
		if e != nil {
			return c, e
		}
		c.Password = string(p)
	}
	c.PasswordSet = c.Password != ""
	return c, nil
}
func (s *Server) handleGetSMTP(w http.ResponseWriter, r *http.Request) {
	c, e := s.loadSMTP(r.Context())
	if e != nil {
		writeError(w, 500, "internal", "cannot load SMTP settings")
		return
	}
	c.Password = ""
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, c)
}
func (s *Server) handlePutSMTP(w http.ResponseWriter, r *http.Request) {
	var c smtpSettings
	if !decodeJSON(w, r, &c) {
		return
	}
	c.Host = strings.TrimSpace(c.Host)
	c.Sender = strings.TrimSpace(c.Sender)
	_, emailErr := mail.ParseAddress(c.Sender)
	if c.Host == "" || len(c.Host) > 253 || strings.ContainsAny(c.Host, " /\\\r\n\t:@") || c.Port < 1 || c.Port > 65535 || emailErr != nil || strings.ContainsAny(c.Sender, "\r\n") || len(c.Sender) > 320 || len(c.Username) > 320 || len(c.Password) > 1024 || (c.TLS != "starttls" && c.TLS != "tls" && c.TLS != "none") || (c.TLS == "none" && c.Username != "") {
		writeError(w, 422, "validation", "invalid SMTP settings; authentication requires TLS")
		return
	}
	old, e := s.loadSMTP(r.Context())
	if e != nil {
		writeError(w, 500, "internal", "cannot load SMTP settings")
		return
	}
	if c.ClearPassword {
		c.Password = ""
	} else if c.Password == "" {
		c.Password = old.Password
	}
	if c.Username != "" && c.Password == "" {
		writeError(w, 422, "validation", "password required")
		return
	}
	enc, e := s.box.Seal([]byte(c.Password))
	if e != nil {
		writeError(w, 500, "internal", "cannot encrypt SMTP password")
		return
	}
	_, e = s.store.Pool().Exec(r.Context(), `INSERT INTO smtp_settings(id,host,port,username,sender,tls_mode,password_enc) VALUES(true,$1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO UPDATE SET host=EXCLUDED.host,port=EXCLUDED.port,username=EXCLUDED.username,sender=EXCLUDED.sender,tls_mode=EXCLUDED.tls_mode,password_enc=EXCLUDED.password_enc,updated_at=now()`, c.Host, c.Port, c.Username, c.Sender, c.TLS, enc)
	if e != nil {
		writeError(w, 500, "internal", "cannot save SMTP settings")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) sendConfiguredMail(ctx context.Context, m mailer.Message) error {
	c, e := s.loadSMTP(ctx)
	if e != nil {
		return e
	}
	if c.Source == "environment" {
		return s.mailer.Send(ctx, m)
	}
	transport := mailer.SMTP{Addr: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), User: c.Username, Password: c.Password, From: c.Sender, TLSMode: c.TLS}
	return transport.Send(ctx, m)
}
