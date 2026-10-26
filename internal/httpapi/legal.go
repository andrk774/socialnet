package httpapi

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/legal"
)

// Юридические документы, согласия и удаление аккаунта.
//
// Регистрация требует трёх отметок: 18+, принятие соглашения и политики и
// отдельное согласие на обработку ПДн. Когда документы меняются (растёт
// legal.Revision), пользователь может читать сайт, но не может ничего менять,
// пока не примет новую редакцию: см. consentGate.

func (s *Server) legalOperator() legal.Operator {
	email := s.cfg.LegalEmail
	if email == "" {
		email = s.cfg.SecurityContact
	}
	return legal.Operator{Name: s.cfg.LegalOperator, INN: s.cfg.LegalINN, Address: s.cfg.LegalAddress,
		Email: email, SiteURL: s.cfg.AppURL}
}

// handleLegalPage: GET /legal/{doc} — документ как обычная HTML-страница
// (без входа и без скриптов: её читают до регистрации и поисковики).
func (s *Server) handleLegalPage(w http.ResponseWriter, r *http.Request) {
	d, ok := legal.ByKey(chi.URLParam(r, "doc"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	blocks, err := legal.Render(d, s.legalOperator())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var b bytes.Buffer
	if err := legal.Page.Execute(&b, map[string]any{"Doc": d, "Blocks": blocks, "All": legal.Docs}); err != nil {
		s.internalError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(b.Bytes())
}

type legalDocView struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

func legalDocs() []legalDocView {
	out := make([]legalDocView, len(legal.Docs))
	for i, d := range legal.Docs {
		out[i] = legalDocView{Key: d.Key, Title: d.Title, Version: d.Version, URL: "/legal/" + d.Key}
	}
	return out
}

// handleLegal: GET /api/v1/legal — список документов и текущая редакция.
func (s *Server) handleLegal(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"revision": legal.Revision, "documents": legalDocs()})
}

// consentRequest — три отдельные отметки; ни одна не ставится за пользователя.
type consentRequest struct {
	AgeConfirmed bool `json:"age_confirmed"` // мне есть 18 лет
	AcceptTerms  bool `json:"accept_terms"`  // принимаю соглашение и политику
	PDConsent    bool `json:"pd_consent"`    // согласие на обработку ПДн
}

func (c consentRequest) validate(errs map[string]string) {
	if !c.AgeConfirmed {
		errs["age_confirmed"] = "the service is for adults (18+)"
	}
	if !c.AcceptTerms {
		errs["accept_terms"] = "accept the terms of use and the privacy policy"
	}
	if !c.PDConsent {
		errs["pd_consent"] = "consent to personal data processing is required"
	}
}

func (s *Server) recordConsent(r *http.Request, user int64) error {
	docs := map[string]string{"age18": "1"}
	for _, d := range legal.Docs {
		docs[d.Key] = d.Version
	}
	return s.store.AcceptLegal(r.Context(), user, legal.Revision, docs, clientIP(r), r.UserAgent())
}

// handleConsents: GET /api/v1/users/me/consents — что принято и надо ли принять заново.
func (s *Server) handleConsents(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	rev, err := s.store.LegalRev(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	log, err := s.store.ConsentLog(r.Context(), me)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"required": rev < legal.Revision, "revision": legal.Revision, "accepted_revision": rev,
		"documents": legalDocs(), "log": log,
	})
}

// handleAcceptConsents: POST /api/v1/users/me/consents — принять текущую редакцию.
func (s *Server) handleAcceptConsents(w http.ResponseWriter, r *http.Request) {
	var req consentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	errs := map[string]string{}
	req.validate(errs)
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := userIDFrom(r.Context())
	if err := s.recordConsent(r, me); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, me, "legal_accepted")
	w.WriteHeader(http.StatusNoContent)
}

// consentGate: пока не приняты действующие документы — только чтение.
// Разрешено: принять документы, удалить аккаунт (это и есть отзыв согласия).
func consentGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := viewerFrom(r.Context())
		if v.NeedsConsent && r.Method != http.MethodGet && r.Method != http.MethodHead {
			p := strings.TrimSuffix(r.URL.Path, "/")
			allowed := p == "/api/v1/users/me/consents" || p == "/api/v1/ws/ticket" ||
				p == "/api/v1/users/me" && r.Method == http.MethodDelete
			if !allowed {
				writeError(w, http.StatusPreconditionRequired, "consent_required",
					"accept the updated terms, privacy policy and personal data consent first")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleDeleteAccount: DELETE /api/v1/users/me {"password"} — удалить аккаунт
// и отозвать согласие на обработку персональных данных.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	v := viewerFrom(r.Context())
	if v.API {
		writeError(w, http.StatusForbidden, "forbidden", "only the account owner can delete it, not an app")
		return
	}
	if !s.checkPassword(w, r, v.ID, req.Password) {
		return
	}
	files, err := s.store.DeleteAccount(r.Context(), v.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	for _, id := range files.Media {
		if err := s.files.Delete(id); err != nil {
			s.log.Warn("delete account media", "err", err, "id", id)
		}
	}
	for kind, ids := range map[string][]string{"docs": files.Documents, "routes": files.Routes, "videos": files.Videos} {
		for _, id := range ids {
			if err := s.avStore.Delete(kind, id); err != nil {
				s.log.Warn("delete account file", "err", err, "kind", kind, "id", id)
			}
		}
	}
	s.log.Info("account deleted", "user_id", v.ID)
	s.m.business.Inc("account_deleted")
	s.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
