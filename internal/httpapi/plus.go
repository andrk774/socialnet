package httpapi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"socialnet/internal/store"
)

// Подписка «Плюс» за монеты. Что даёт:
//   - выше лимиты: свои ленты 5→20, открытые объявления о попутчиках 5→15,
//     статьи в сутки 10→30, офлайн-тайлы на маршрут 600→2000;
//   - значок «Плюс» в профиле;
//   - клубам (владелец сообщества с «Плюсом»): эмодзи 50→200; организатору
//     похода — список участников CSV для регистрации группы в МЧС.
// Оплата — обычная проводка кошелька с ключом идемпотентности; продление —
// раз в период, тем же механизмом.

const plusDays = 30

// plusLimit: лимит с учётом подписки. Ошибка проверки — обычный лимит.
func (s *Server) plusLimit(r *http.Request, user int64, base, plus int) int {
	ok, err := s.store.HasPlus(r.Context(), user)
	if err != nil {
		s.log.Warn("plus check", "err", err)
		return base
	}
	if ok {
		return plus
	}
	return base
}

func (s *Server) plusInfo(p store.Plus) map[string]any {
	return map[string]any{
		"active": p.Active, "expires_at": p.ExpiresAt, "auto_renew": p.AutoRenew,
		"price": s.cfg.PlusPrice, "days": plusDays,
		"limits": map[string]any{
			"antennas":      [2]int{store.MaxAntennas, store.MaxAntennasPlus},
			"companion_ads": [2]int{store.MaxOpenCompanionAds, store.MaxOpenCompanionAdsPlus},
			"articles_day":  [2]int{store.MaxArticlesPerDay, store.MaxArticlesPerDayPlus},
			"offline_tiles": [2]int{600, 2000},
			"club_emoji":    [2]int{store.MaxEmojiPerCommunity, store.MaxEmojiPerCommunityPlus},
		},
	}
}

func (s *Server) handlePlus(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.PlusStatus(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.plusInfo(p))
}

// handleBuyPlus: POST /plus/buy {"idempotency_key"} — на plusDays дней.
func (s *Server) handleBuyPlus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		key = r.Header.Get("Idempotency-Key")
	}
	if len(key) < 8 || len(key) > 64 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"idempotency_key": "8-64 characters (a UUID per purchase)"})
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	rec, p, err := s.store.BuyPlus(r.Context(), me.ID, s.cfg.PlusPrice, plusDays, key)
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !rec.Replayed {
		s.m.business.Inc("plus_bought")
	}
	out := s.plusInfo(p)
	out["replayed"] = rec.Replayed
	bal, err := s.store.Balance(r.Context(), store.UserAccount(me.ID))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out["balance"] = bal
	code := http.StatusCreated
	if rec.Replayed {
		code = http.StatusOK
	}
	writeJSON(w, code, out)
}

func (s *Server) handlePlusSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoRenew *bool `json:"auto_renew"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AutoRenew == nil {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", map[string]string{"auto_renew": "required"})
		return
	}
	me := userIDFrom(r.Context())
	if err := s.store.SetPlusAutoRenew(r.Context(), me, *req.AutoRenew); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no subscription")
		return
	} else if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.handlePlus(w, r)
}

// RunPlusRenewals продлевает подписки с автопродлением. Ключ идемпотентности
// привязан к дате окончания: за один период — одно списание, сколько бы
// экземпляров API ни запускали продление.
func (s *Server) RunPlusRenewals(ctx context.Context) error {
	ids, err := s.store.DueRenewals(ctx, 200)
	if err != nil {
		return err
	}
	for _, u := range ids {
		p, err := s.store.PlusStatus(ctx, u)
		if err != nil || p.ExpiresAt == nil {
			return err
		}
		key := fmt.Sprintf("plus-renew-%s", p.ExpiresAt.UTC().Format("20060102T1504"))
		_, _, err = s.store.BuyPlus(ctx, u, s.cfg.PlusPrice, plusDays, key)
		switch {
		case errors.Is(err, store.ErrInsufficientFunds), errors.Is(err, store.ErrWalletDebt):
			if err := s.store.SetPlusAutoRenew(ctx, u, false); err != nil {
				return err
			}
			s.notify(ctx, store.NewNotification{UserID: u, Type: "plus_renew_failed"})
		case err != nil:
			return err
		default:
			s.notify(ctx, store.NewNotification{UserID: u, Type: "plus_renewed"})
		}
	}
	return nil
}

// csvCell защищает от формул при открытии в Excel/Calc (CSV injection).
func csvCell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// handleParticipantsCSV: GET /events/{id}/participants.csv — организатору с
// «Плюсом»: список группы для регистрации в МЧС.
func (s *Server) handleParticipantsCSV(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	if plus, err := s.store.HasPlus(r.Context(), me); err != nil {
		s.internalError(w, r, err)
		return
	} else if !plus {
		writeError(w, http.StatusPaymentRequired, "plus_required", "participant export is a «Плюс» feature")
		return
	}
	var rows [][]string
	for _, st := range []string{"going", "waitlist"} {
		var after int64
		for {
			list, err := s.store.Attendees(r.Context(), e.ID, st, after, 100)
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			for _, u := range list {
				label := "идёт"
				if st == "waitlist" {
					label = "в очереди"
				}
				rows = append(rows, []string{csvCell(u.FirstName), csvCell(u.LastName), csvCell(u.Username), label})
				after = u.ID
			}
			if len(list) < 100 {
				break
			}
		}
	}
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", contentDisposition(fmt.Sprintf("участники-%d-%s.csv", e.ID, e.StartsAt.In(uralTime).Format("2006-01-02"))))
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // BOM: Excel откроет кириллицу правильно
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Имя", "Фамилия", "Ник", "Статус"})
	_ = cw.WriteAll(rows)
}
