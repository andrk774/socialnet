package httpapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type turnSettings struct {
	Enabled   bool   `json:"enabled"`
	STUN      string `json:"stun_url"`
	TURN      string `json:"turn_url"`
	Secret    string `json:"secret,omitempty"`
	SecretSet bool   `json:"secret_set"`
	Source    string `json:"source"`
}

func (s *Server) loadTURN(ctx context.Context) (turnSettings, error) {
	c := turnSettings{}
	var enc []byte
	err := s.store.Pool().QueryRow(ctx, `SELECT enabled,stun_url,turn_url,secret_enc FROM turn_settings WHERE id=true`).Scan(&c.Enabled, &c.STUN, &c.TURN, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		c.STUN = os.Getenv("CALLS_STUN_URL")
		c.TURN = os.Getenv("CALLS_TURN_URL")
		c.Secret = os.Getenv("CALLS_TURN_SECRET")
		c.Enabled = c.TURN != "" && c.Secret != ""
		c.Source = "environment"
	} else if err != nil {
		return c, err
	} else {
		c.Source = "database"
		if len(enc) > 0 {
			plain, e := s.box.Open(enc)
			if e != nil {
				return c, e
			}
			c.Secret = string(plain)
		}
	}
	c.SecretSet = c.Secret != ""
	return c, nil
}

func validICE(raw string, turn bool) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 512 || strings.ContainsAny(raw, " \r\n\t/#@") {
		return false
	}
	scheme, rest, ok := strings.Cut(raw, ":")
	if !ok {
		return false
	}
	if turn {
		if scheme != "turn" && scheme != "turns" {
			return false
		}
	} else if scheme != "stun" {
		return false
	}
	u, err := url.Parse("https://" + rest)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" {
		return false
	}
	if u.RawQuery != "" && (!turn || (u.RawQuery != "transport=udp" && u.RawQuery != "transport=tcp")) {
		return false
	}
	return true
}

func (s *Server) handleGetTURN(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadTURN(r.Context())
	if err != nil {
		writeError(w, 500, "internal", "cannot load TURN settings")
		return
	}
	c.Secret = ""
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, c)
}

func (s *Server) handlePutTURN(w http.ResponseWriter, r *http.Request) {
	var c turnSettings
	if !decodeJSON(w, r, &c) {
		return
	}
	c.STUN = strings.TrimSpace(c.STUN)
	c.TURN = strings.TrimSpace(c.TURN)
	if !validICE(c.STUN, false) || !validICE(c.TURN, true) || len(c.Secret) > 512 {
		writeError(w, 422, "validation", "invalid ICE URL or secret")
		return
	}
	old, err := s.loadTURN(r.Context())
	if err != nil {
		writeError(w, 500, "internal", "cannot load TURN settings")
		return
	}
	if c.Secret == "" {
		c.Secret = old.Secret
	}
	if c.Enabled && (c.TURN == "" || len(c.Secret) < 16) {
		writeError(w, 422, "validation", "TURN URL and secret of at least 16 characters required")
		return
	}
	enc, err := s.box.Seal([]byte(c.Secret))
	if err != nil {
		writeError(w, 500, "internal", "cannot encrypt secret")
		return
	}
	_, err = s.store.Pool().Exec(r.Context(), `INSERT INTO turn_settings(id,enabled,stun_url,turn_url,secret_enc) VALUES(true,$1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET enabled=EXCLUDED.enabled,stun_url=EXCLUDED.stun_url,turn_url=EXCLUDED.turn_url,secret_enc=EXCLUDED.secret_enc,updated_at=now()`, c.Enabled, c.STUN, c.TURN, enc)
	if err != nil {
		writeError(w, 500, "internal", "cannot save TURN settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
