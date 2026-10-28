package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/qr"
)

// QR-код похода ведёт на короткий адрес /qr/e/{id}: сервер считает скан и
// перенаправляет на страницу похода с ?from=qr, чтобы запись в поход после
// этого тоже попала в статистику. Личных данных не собираем: только
// счётчики по дням; повторный скан из того же браузера в течение часа не
// считается, превью ссылок в мессенджерах — тоже.

func (s *Server) qrTarget(id int64) string {
	return s.cfg.AppURL + "/qr/e/" + strconv.FormatInt(id, 10)
}

// handleEventQR: GET /api/v1/events/{id}/qr.svg — QR-код похода картинкой.
func (s *Server) handleEventQR(w http.ResponseWriter, r *http.Request) {
	e, ok := s.eventParam(w, r)
	if !ok {
		return
	}
	code, err := qr.Encode([]byte(s.qrTarget(e.ID)), qr.M)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Content-Disposition", contentDisposition("qr-"+icsFileName(e.Title)+".svg"))
	h.Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write([]byte(code.SVG("#0A4A3A", "#ffffff")))
}

// handleEventQRStats: GET /api/v1/events/{id}/qr-stats — организатору.
func (s *Server) handleEventQRStats(w http.ResponseWriter, r *http.Request) {
	e, ok := s.managedEvent(w, r)
	if !ok {
		return
	}
	st, err := s.store.QRStats(r.Context(), e.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleQRScan: GET /qr/e/{id} — без входа: посчитать и перенаправить.
func (s *Server) handleQRScan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	cookie := "qr_e" + strconv.FormatInt(id, 10)
	_, seen := r.Cookie(cookie)
	if seen != nil && !linkPreviewBot(r.UserAgent()) && s.publicLimiter.allow(clientIP(r)) {
		exists, err := s.store.RecordQRScan(r.Context(), id)
		if err != nil {
			s.log.Error("qr scan", "err", err)
		} else if exists {
			s.m.business.Inc("qr_scan")
			// #nosec G124 -- HttpOnly и SameSite=Lax всегда (Lax: переход по QR — это переход с другого «сайта»); Secure выключается только COOKIE_SECURE=false для разработки по http
			http.SetCookie(w, &http.Cookie{Name: cookie, Value: "1", Path: "/qr/", MaxAge: int(time.Hour / time.Second),
				HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/events/"+strconv.FormatInt(id, 10)+"?from=qr", http.StatusFound)
}

// linkPreviewBot: Telegram, VK, WhatsApp и поисковики открывают ссылку ради
// превью — это не скан.
func linkPreviewBot(ua string) bool {
	ua = strings.ToLower(ua)
	for _, s := range []string{"bot", "crawler", "spider", "preview", "facebookexternalhit", "whatsapp", "vkshare", "skypeuripreview"} {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return ua == ""
}
