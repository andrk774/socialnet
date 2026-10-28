package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/auth"
	"socialnet/internal/qr"
	"socialnet/internal/store"
)

// Вход по QR-коду, как в Telegram Web:
//
//  1. Новое устройство: POST /auth/qr/start → id (в QR-коде) и секрет опроса.
//  2. Телефон, где вы уже вошли, сканирует код, видит, какое устройство
//     просит вход (браузер, IP), и подтверждает: POST /qr-login/{id}/approve.
//  3. Новое устройство опрашивает POST /auth/qr/poll {id, poll_token} и после
//     подтверждения получает обычный сеанс — как после входа по паролю.
//
// Защита: код живёт 2 минуты и срабатывает один раз; забрать сеанс можно
// только с секретом опроса (его нет в QR-коде — фото экрана не поможет);
// вход подтверждает человек, а не сам факт сканирования; приложения с
// токенами API подтверждать вход не могут. Подмена кода (QRLjacking:
// злоумышленник подсовывает свой код) закрывается тем, что телефон
// показывает устройство и предупреждает не подтверждать чужие коды.

// handleQRLoginStart: POST /auth/qr/start — без входа.
func (s *Server) handleQRLoginStart(w http.ResponseWriter, r *http.Request) {
	id, _ := randomToken("", 18)
	poll, pollHash := randomToken("", 32) // в базе — только хэш секрета
	exp, err := s.store.CreateQRLogin(r.Context(), id, pollHash, sessionMeta(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	link := s.cfg.AppURL + "/qr-login?c=" + id
	code, err := qr.Encode([]byte(link), qr.M)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "poll_token": poll, "url": link, "expires_at": exp, "qr_svg": code.SVG("#0A4A3A", "#ffffff"),
	})
}

// handleQRLoginPoll: POST /auth/qr/poll {id, poll_token} — новое устройство ждёт подтверждения.
func (s *Server) handleQRLoginPoll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string `json:"id"`
		PollToken string `json:"poll_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID == "" || req.PollToken == "" || len(req.ID) > 64 || len(req.PollToken) > 100 {
		writeError(w, http.StatusGone, "qr_expired", "the code has expired, get a new one")
		return
	}
	userID, err := s.store.ClaimQRLogin(r.Context(), req.ID, auth.HashRefreshToken(req.PollToken))
	switch {
	case errors.Is(err, store.ErrQRPending):
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
		return
	case errors.Is(err, store.ErrQRDenied):
		writeError(w, http.StatusForbidden, "qr_rejected", "the login was rejected on the phone")
		return
	case errors.Is(err, store.ErrQRExpired):
		writeError(w, http.StatusGone, "qr_expired", "the code has expired, get a new one")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	user, err := s.store.UserByID(r.Context(), userID)
	if err != nil || user.BannedAt != nil {
		writeError(w, http.StatusForbidden, "banned", "account is banned")
		return
	}
	s.m.business.Inc("qr_login")
	s.finishLogin(w, r, user)
}

// handleQRLoginInfo: GET /qr-login/{id} — телефону: какое устройство просит вход.
func (s *Server) handleQRLoginInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.store.QRLoginInfo(r.Context(), qrLoginID(r))
	if errors.Is(err, store.ErrQRExpired) {
		writeError(w, http.StatusGone, "qr_expired", "the code has expired or was already used")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleQRLoginDecide: POST /qr-login/{id}/approve и /reject.
func (s *Server) handleQRLoginDecide(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := userIDFrom(r.Context())
		err := s.store.DecideQRLogin(r.Context(), qrLoginID(r), me, approve)
		if errors.Is(err, store.ErrQRExpired) {
			writeError(w, http.StatusGone, "qr_expired", "the code has expired or was already used")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if approve {
			s.event(r, me, "qr_login_approved")
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func qrLoginID(r *http.Request) string {
	id := chi.URLParam(r, "id")
	if len(id) > 64 || strings.ContainsAny(id, "/?#") {
		return ""
	}
	return id
}
