package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"socialnet/internal/store"
)

const (
	maxTransfer = 1_000_000
	dailyBonus  = 5
)

// moneyRequest — общие поля денежных операций.
type moneyRequest struct {
	Amount         int64  `json:"amount"`
	Memo           string `json:"memo"`
	IdempotencyKey string `json:"idempotency_key"`
}

// validate проверяет сумму и ключ идемпотентности. Ключ можно передать
// и заголовком Idempotency-Key. Клиент генерирует его (UUID) один раз на
// операцию и повторяет при сетевых ошибках — деньги не спишутся дважды.
func (m *moneyRequest) validate(r *http.Request) map[string]string {
	errs := map[string]string{}
	if m.IdempotencyKey == "" {
		m.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	m.Memo = strings.TrimSpace(m.Memo)
	if m.Amount < 1 || m.Amount > maxTransfer {
		errs["amount"] = "1-1000000 coins"
	}
	if utf8.RuneCountInString(m.Memo) > 200 {
		errs["memo"] = "max 200 characters"
	}
	if k := m.IdempotencyKey; k == "" || len(k) > 64 {
		errs["idempotency_key"] = "required, up to 64 characters (use a UUID)"
	}
	return errs
}

// moneyError переводит ошибки кошелька в ответы API.
func (s *Server) moneyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrWalletDebt):
		writeError(w, http.StatusConflict, "wallet_debt", "a refunded purchase left a debt; spending is blocked until it is repaid")
	case errors.Is(err, store.ErrInsufficientFunds):
		writeError(w, http.StatusPaymentRequired, "insufficient_funds", "not enough coins")
	case errors.Is(err, store.ErrIdempotencyReuse):
		writeError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", "this idempotency key was used for a different operation")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	default:
		s.internalError(w, r, err)
	}
}

// requireVerified: тратить монеты можно только с подтверждённым email —
// иначе бонусы легко фармить пачкой фейковых аккаунтов.
func (s *Server) requireVerified(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	user, err := s.store.UserByID(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if user.EmailVerifiedAt == nil {
		writeError(w, http.StatusForbidden, "email_not_verified", "confirm your email to use the wallet")
		return nil, false
	}
	if !s.walletLimiter.allow(strconv.FormatInt(user.ID, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many operations, slow down")
		return nil, false
	}
	return user, true
}

func txCursor(t store.TxView) int64 { return t.ID }

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	acc := store.UserAccount(userIDFrom(r.Context()))
	bal, err := s.store.Balance(r.Context(), acc)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p := pageParams(r)
	hist, err := s.store.History(r.Context(), acc, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	debt, err := s.store.Debt(r.Context(), acc.Owner)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": bal, "debt": debt, "transactions": list(hist, p.Limit, txCursor)})
}

func (s *Server) handleTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		moneyRequest
		ToUserID int64 `json:"to_user_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(r); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	if req.ToUserID == me.ID {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot transfer to yourself")
		return
	}
	to, err := s.store.UserByID(r.Context(), req.ToUserID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && to.BannedAt != nil) {
		writeError(w, http.StatusNotFound, "not_found", "recipient not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rel, err := s.store.Relationship(r.Context(), me.ID, to.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if rel.Blocked() {
		writeError(w, http.StatusForbidden, "blocked", "action unavailable")
		return
	}

	rec, err := s.store.Transfer(r.Context(), store.Transfer{Kind: "transfer",
		From: store.UserAccount(me.ID), To: store.UserAccount(to.ID),
		Amount: req.Amount, Memo: req.Memo, Initiator: me.ID, IdemKey: req.IdempotencyKey})
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !rec.Replayed {
		s.notify(r.Context(), store.NewNotification{UserID: to.ID, Type: "transfer", ActorID: me.ID})
	}
	s.respondTx(w, r, rec, store.UserAccount(me.ID))
}

func (s *Server) respondTx(w http.ResponseWriter, r *http.Request, rec *store.TxRecord, acc store.Account) {
	bal, err := s.store.Balance(r.Context(), acc)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	status := http.StatusCreated
	if rec.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"transaction_id": rec.ID, "amount": rec.Amount, "replayed": rec.Replayed, "balance": bal,
	})
}

func (s *Server) handleDailyBonus(w http.ResponseWriter, r *http.Request) {
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	rec, err := s.store.DailyBonus(r.Context(), me.ID, dailyBonus)
	if errors.Is(err, store.ErrAlreadyClaimed) {
		writeError(w, http.StatusConflict, "already_claimed", "come back tomorrow")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.respondTx(w, r, rec, store.UserAccount(me.ID))
}

// ---------- подарки ----------

func (s *Server) handleGiftCatalog(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.GiftCatalog(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSendGift(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var req struct {
		GiftID         int    `json:"gift_id"`
		Message        string `json:"message"`
		Anonymous      bool   `json:"anonymous"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	req.Message = strings.TrimSpace(req.Message)
	errs := map[string]string{}
	if utf8.RuneCountInString(req.Message) > 200 {
		errs["message"] = "max 200 characters"
	}
	if k := req.IdempotencyKey; k == "" || len(k) > 64 {
		errs["idempotency_key"] = "required, up to 64 characters (use a UUID)"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	if target.ID == me.ID {
		writeError(w, http.StatusBadRequest, "bad_request", "cannot send a gift to yourself")
		return
	}
	if rel.Blocked() {
		writeError(w, http.StatusForbidden, "blocked", "action unavailable")
		return
	}
	id, replayed, err := s.store.SendGift(r.Context(), me.ID, target.ID, req.GiftID, req.Message, req.Anonymous, req.IdempotencyKey)
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !replayed {
		actor := me.ID
		if req.Anonymous {
			actor = 0 // в уведомлении отправитель не указывается
		}
		s.notify(r.Context(), store.NewNotification{UserID: target.ID, Type: "gift", ActorID: actor})
	}
	bal, err := s.store.Balance(r.Context(), store.UserAccount(me.ID))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"gift_id": id, "replayed": replayed, "balance": bal})
}

func (s *Server) handleUserGifts(w http.ResponseWriter, r *http.Request) {
	target, rel, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if !canSee(target.ProfileVisibility, target.ID == userIDFrom(r.Context()), rel) {
		writeError(w, http.StatusForbidden, "private", "profile is closed")
		return
	}
	p := pageParams(r)
	items, err := s.store.UserGifts(r.Context(), target.ID, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, func(g store.SentGift) int64 { return g.ID }))
}

// ---------- сообщества ----------

func (s *Server) handleDonate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	var req moneyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(r); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	rec, err := s.store.Transfer(r.Context(), store.Transfer{Kind: "donation",
		From: store.UserAccount(me.ID), To: store.CommunityAccount(c.ID),
		Amount: req.Amount, Memo: req.Memo, Initiator: me.ID, IdemKey: req.IdempotencyKey})
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !rec.Replayed {
		s.notify(r.Context(), store.NewNotification{UserID: c.OwnerID, Type: "donation", ActorID: me.ID})
	}
	s.respondTx(w, r, rec, store.UserAccount(me.ID))
}

// Кошелёк сообщества видят админы и владелец.
func (s *Server) handleCommunityWallet(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	if !store.RoleAtLeast(c.MyRole, "admin") {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can view the community wallet")
		return
	}
	acc := store.CommunityAccount(c.ID)
	bal, err := s.store.Balance(r.Context(), acc)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p := pageParams(r)
	hist, err := s.store.History(r.Context(), acc, p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	debt, err := s.store.Debt(r.Context(), acc.Owner)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": bal, "debt": debt, "transactions": list(hist, p.Limit, txCursor)})
}

// Выводить монеты сообщества может только владелец и только участнику.
func (s *Server) handleCommunityPayout(w http.ResponseWriter, r *http.Request) {
	c, ok := s.community(w, r)
	if !ok {
		return
	}
	var req struct {
		moneyRequest
		ToUserID int64 `json:"to_user_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(r); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if c.MyRole != "owner" {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner can pay out")
		return
	}
	me, ok := s.requireVerified(w, r)
	if !ok {
		return
	}
	role, err := s.store.CommunityRole(r.Context(), c.ID, req.ToUserID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if role == "" {
		writeError(w, http.StatusNotFound, "not_found", "recipient is not a member")
		return
	}
	rec, err := s.store.Transfer(r.Context(), store.Transfer{Kind: "payout",
		From: store.CommunityAccount(c.ID), To: store.UserAccount(req.ToUserID),
		Amount: req.Amount, Memo: req.Memo, Initiator: me.ID, IdemKey: req.IdempotencyKey})
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	s.respondTx(w, r, rec, store.CommunityAccount(c.ID))
}

// handleMint (только админ): начислить монеты из системного счёта.
func (s *Server) handleMint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		moneyRequest
		UserID int64 `json:"user_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := req.validate(r); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if _, err := s.store.UserByID(r.Context(), req.UserID); err != nil {
		s.moneyError(w, r, err)
		return
	}
	me := userIDFrom(r.Context())
	rec, err := s.store.Transfer(r.Context(), store.Transfer{Kind: "mint",
		From: store.SystemAccount, To: store.UserAccount(req.UserID),
		Amount: req.Amount, Memo: req.Memo, Initiator: me, IdemKey: req.IdempotencyKey})
	if err != nil {
		s.moneyError(w, r, err)
		return
	}
	if !rec.Replayed {
		s.modLog(r, "mint:"+strconv.FormatInt(req.Amount, 10), "user", req.UserID, nil, req.Memo)
	}
	s.respondTx(w, r, rec, store.UserAccount(req.UserID))
}
