package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/store"
	"socialnet/internal/yookassa"
)

type coinPackage struct {
	ID      string `json:"id"`
	Coins   int64  `json:"coins"`
	Kopecks int64  `json:"price_kopecks"`
	Price   string `json:"price"` // "99.00 ₽" — для показа
}

// Пакеты монет. Цена фиксируется в строке платежа в момент покупки.
var coinPackages = func() []coinPackage {
	ps := []coinPackage{{ID: "coins_100", Coins: 100, Kopecks: 9900}, {ID: "coins_550", Coins: 550, Kopecks: 49900},
		{ID: "coins_1200", Coins: 1200, Kopecks: 99900}}
	for i := range ps {
		ps[i].Price = yookassa.RUB(ps[i].Kopecks).Value + " ₽"
	}
	return ps
}()

func findPackage(id string) (coinPackage, bool) {
	for _, p := range coinPackages {
		if p.ID == id {
			return p, true
		}
	}
	return coinPackage{}, false
}

func (s *Server) paymentsEnabled(w http.ResponseWriter) bool {
	if s.yookassa == nil {
		writeError(w, http.StatusServiceUnavailable, "payments_disabled", "buying coins is not configured on this server")
		return false
	}
	return true
}

func (s *Server) handleCoinPackages(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": coinPackages, "enabled": s.yookassa != nil})
}

func (s *Server) handleCreatePurchase(w http.ResponseWriter, r *http.Request) {
	if !s.paymentsEnabled(w) {
		return
	}
	var req struct {
		PackageID string `json:"package_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	pkg, ok := findPackage(req.PackageID)
	if !ok {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"package_id": "unknown package"})
		return
	}
	me := userIDFrom(r.Context())
	if !s.walletLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many wallet operations")
		return
	}
	user, err := s.store.UserByID(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Чек по 54-ФЗ уходит на email: он должен быть подтверждён.
	if user.EmailVerifiedAt == nil {
		writeError(w, http.StatusForbidden, "email_unverified", "verify your email to receive the receipt")
		return
	}
	pay, err := s.store.CreatePayment(r.Context(), me, pkg.ID, pkg.Coins, pkg.Kopecks, user.Email)
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusTooManyRequests, "too_many_pending", "finish or cancel your previous purchases first")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	item := yookassa.ReceiptItem{Description: fmt.Sprintf("Внутренняя валюта Socialnet, %d монет", pkg.Coins),
		Quantity: "1.00", Amount: yookassa.RUB(pkg.Kopecks), VATCode: s.cfg.YooKassaVATCode,
		PaymentMode: "full_payment", PaymentSubject: "service", Measure: "piece"}
	receipt := &yookassa.Receipt{Items: []yookassa.ReceiptItem{item}}
	receipt.Customer.Email = user.Email
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	remote, err := s.yookassa.CreatePayment(ctx, yookassa.CreateRequest{
		Amount: yookassa.RUB(pkg.Kopecks), Capture: true,
		Confirmation: yookassa.Confirmation{Type: "redirect", ReturnURL: s.cfg.AppURL + "/wallet?payment=" + pay.ID},
		Description:  fmt.Sprintf("%d монет, заказ %s", pkg.Coins, pay.ID),
		Metadata:     map[string]string{"payment_id": pay.ID, "user_id": strconv.FormatInt(me, 10)},
		Receipt:      receipt,
	}, pay.ID) // наш id — ключ идемпотентности: повтор запроса не создаст второй платёж
	if err == nil && (remote.ID == "" || remote.Confirmation == nil || !strings.HasPrefix(remote.Confirmation.ConfirmationURL, "https://")) {
		err = errors.New("unexpected response: no https confirmation_url")
	}
	if err != nil {
		s.log.Warn("yookassa create payment", "err", err, "payment", pay.ID)
		_ = s.store.PaymentFailed(r.Context(), pay.ID, err.Error())
		writeError(w, http.StatusBadGateway, "payment_provider_error", "payment provider is unavailable, try again later")
		return
	}
	if err := s.store.PaymentCreated(r.Context(), pay.ID, remote.ID, remote.Confirmation.ConfirmationURL); err != nil {
		s.internalError(w, r, err)
		return
	}
	pay, err = s.store.OwnPayment(r.Context(), me, pay.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, pay)
}

// syncPayment сверяет платёж с ЮKassa и, если оплачен, начисляет монеты.
// Сумма, валюта и наш номер заказа в metadata должны совпасть — иначе
// это чужой или подменённый платёж.
func (s *Server) syncPayment(ctx context.Context, p *store.Payment) error {
	if p.Status != "pending" || p.ProviderPaymentID == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	remote, err := s.yookassa.GetPayment(ctx, *p.ProviderPaymentID)
	if err != nil {
		return err
	}
	kop, err := yookassa.ParseKopecks(remote.Amount.Value)
	if err != nil || remote.ID != *p.ProviderPaymentID || kop != p.AmountKopecks ||
		remote.Amount.Currency != p.Currency || remote.Metadata["payment_id"] != p.ID {
		s.log.Error("yookassa payment mismatch", "payment", p.ID, "remote", remote.ID, "amount", remote.Amount.Value)
		return errors.New("payment data mismatch")
	}
	if remote.Status == "succeeded" && !remote.Paid {
		return errors.New("succeeded but not paid")
	}
	credited, err := s.store.SettlePayment(ctx, p.ID, remote.Status)
	if err != nil {
		return err
	}
	if credited {
		s.m.business.Inc("coins_purchased")
		s.m.business.Add(uint64(p.Coins), "coins_purchased_amount")
		s.log.Info("coins purchased", "user_id", p.UserID, "coins", p.Coins, "payment", p.ID)
		s.hub.Publish(p.UserID, "payment", map[string]any{"id": p.ID, "status": "succeeded", "coins": p.Coins})
	}
	return nil
}

func (s *Server) handleGetPurchase(w http.ResponseWriter, r *http.Request) {
	if !s.paymentsEnabled(w) {
		return
	}
	id := chi.URLParam(r, "pid")
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "payment not found")
		return
	}
	me := userIDFrom(r.Context())
	p, err := s.store.OwnPayment(r.Context(), me, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "payment not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	// Пользователь вернулся со страницы оплаты раньше уведомления — проверим сами.
	if err := s.syncPayment(r.Context(), p); err != nil {
		s.log.Warn("sync payment", "err", err, "payment", p.ID)
	} else if p, err = s.store.OwnPayment(r.Context(), me, id); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleMyPurchases(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.MyPayments(r.Context(), userIDFrom(r.Context()), 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// handleYooKassaWebhook. Уведомления ЮKassa не подписаны, поэтому:
// 1) принимаем только с её адресов; 2) содержимому не верим вовсе —
// берём из него лишь id и запрашиваем актуальный платёж через API.
func (s *Server) handleYooKassaWebhook(w http.ResponseWriter, r *http.Request) {
	if s.yookassa == nil {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if s.cfg.YooKassaCheckIP {
		ip, err := netip.ParseAddr(clientIP(r))
		if err != nil || !yookassa.FromYooKassa(ip) {
			s.log.Warn("yookassa webhook from untrusted address", "ip", clientIP(r))
			writeError(w, http.StatusForbidden, "forbidden", "untrusted source")
			return
		}
	}
	var n yookassa.Notification
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&n); err != nil || n.Type != "notification" {
		writeError(w, http.StatusBadRequest, "bad_request", "not a notification")
		return
	}
	if !strings.HasPrefix(n.Event, "payment.") {
		w.WriteHeader(http.StatusOK) // возвраты и прочее пока не обрабатываем
		return
	}
	p, err := s.store.PaymentByProviderID(r.Context(), n.Object.ID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusOK) // не наш платёж — повторять бессмысленно
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.syncPayment(r.Context(), p); err != nil {
		// не 200 — ЮKassa повторит уведомление
		s.log.Warn("yookassa webhook sync", "err", err, "payment", p.ID)
		writeError(w, http.StatusBadGateway, "retry", "temporary error")
		return
	}
	w.WriteHeader(http.StatusOK)
}
