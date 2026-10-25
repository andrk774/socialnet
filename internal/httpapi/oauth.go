package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"socialnet/internal/auth"
	"socialnet/internal/store"
)

// OAuth 2.0 (RFC 6749) в профиле OAuth 2.1: только authorization code,
// PKCE S256 обязателен для всех клиентов, redirect_uri сравнивается точно,
// код одноразовый, в ответе есть iss (RFC 9207) против подмены сервера.

const (
	authCodeTTL    = 10 * time.Minute
	oauthTokenTTL  = 24 * time.Hour
	maxAppRedirect = 500
)

func randomToken(prefix string, n int) (string, []byte) {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read не возвращает ошибок с Go 1.24
	tok := prefix + base64.RawURLEncoding.EncodeToString(b)
	return tok, auth.HashRefreshToken(tok)
}

var customSchemeRe = regexp.MustCompile(`^[a-z][a-z0-9+\-]*(\.[a-z0-9+\-]+)+$`)

// validRedirect: https; http только для loopback (RFC 8252 §7.3);
// приватная схема мобильного приложения вида com.example.app:/cb (§7.1).
func validRedirect(raw string) bool {
	if raw == "" || len(raw) > maxAppRedirect || strings.ContainsAny(raw, " \t\r\n#") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		h := u.Hostname()
		return h == "127.0.0.1" || h == "::1" || h == "localhost"
	default:
		return customSchemeRe.MatchString(u.Scheme) && u.Host == ""
	}
}

type appView struct {
	*store.App
	ClientSecret string `json:"client_secret,omitempty"` // показывается один раз
}

func validateApp(name *string, uris []string) map[string]string {
	errs := map[string]string{}
	if name != nil {
		*name = cleanText(*name, 100)
		if *name == "" {
			errs["name"] = "1-100 characters"
		}
	}
	if uris != nil {
		if len(uris) == 0 || len(uris) > 10 {
			errs["redirect_uris"] = "1-10 redirect URIs"
		}
		for _, u := range uris {
			if !validRedirect(u) {
				errs["redirect_uris"] = "each must be https://, http://127.0.0.1 (loopback) or an app scheme like com.example.app:/cb, without fragment"
			}
		}
	}
	return errs
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
		Confidential bool     `json:"confidential"` // есть сервер, который хранит секрет
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RedirectURIs == nil {
		req.RedirectURIs = []string{}
	}
	if errs := validateApp(&req.Name, req.RedirectURIs); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	me := viewerFrom(r.Context())
	if me.Bot != 0 {
		writeError(w, http.StatusForbidden, "forbidden", "bots cannot create apps")
		return
	}
	idb := make([]byte, 12)
	_, _ = rand.Read(idb)
	var secret string
	var hash []byte
	if req.Confidential {
		secret, hash = randomToken("sns_", 32)
	}
	app, err := s.store.CreateApp(r.Context(), me.ID, req.Name, hex.EncodeToString(idb), hash, req.RedirectURIs)
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, "limit", "at most 20 apps per user")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, me.ID, "app_created")
	writeJSON(w, http.StatusCreated, appView{App: app, ClientSecret: secret})
}

func (s *Server) ownApp(w http.ResponseWriter, r *http.Request) (*store.App, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	app, err := s.store.OwnApp(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "app not found")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	return app, true
}

func (s *Server) handleMyApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.MyApps(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": apps})
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	if app, ok := s.ownApp(w, r); ok {
		writeJSON(w, http.StatusOK, app)
	}
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	app, ok := s.ownApp(w, r)
	if !ok {
		return
	}
	var req struct {
		Name         *string  `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if errs := validateApp(req.Name, req.RedirectURIs); len(errs) > 0 {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input", errs)
		return
	}
	if err := s.store.UpdateApp(r.Context(), app.ID, req.Name, req.RedirectURIs); err != nil {
		s.internalError(w, r, err)
		return
	}
	app, _ = s.store.OwnApp(r.Context(), app.OwnerID, app.ID)
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleRotateAppSecret(w http.ResponseWriter, r *http.Request) {
	app, ok := s.ownApp(w, r)
	if !ok {
		return
	}
	if !app.Confidential {
		writeError(w, http.StatusConflict, "public_client", "public clients have no secret")
		return
	}
	secret, hash := randomToken("sns_", 32)
	if err := s.store.SetAppSecret(r.Context(), app.ID, hash); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, app.OwnerID, "app_secret_rotated")
	writeJSON(w, http.StatusOK, map[string]string{"client_secret": secret})
}

func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	app, ok := s.ownApp(w, r)
	if !ok {
		return
	}
	// токены удаляются каскадом: все пользователи приложения разлогиниваются
	if err := s.store.DeleteApp(r.Context(), app.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- согласие пользователя ----------

type authorizeParams struct {
	ResponseType        string `json:"response_type"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Approve             bool   `json:"approve"`
}

var pkceRe = regexp.MustCompile(`^[A-Za-z0-9\-._~]{43,128}$`)

// checkAuthorize. Пока redirect_uri не проверен, ошибку нельзя отправлять
// на него редиректом (открытый редирект) — только JSON.
func (s *Server) checkAuthorize(w http.ResponseWriter, r *http.Request, p authorizeParams) (*store.App, []string, bool) {
	app, err := s.store.AppByClientID(r.Context(), p.ClientID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return nil, nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, nil, false
	}
	if !slices.Contains(app.RedirectURIs, p.RedirectURI) {
		writeError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri is not registered for this app")
		return nil, nil, false
	}
	errs := map[string]string{}
	if p.ResponseType != "code" {
		errs["response_type"] = "only code is supported"
	}
	scopes, bad := parseScopes(p.Scope, nil)
	if bad != "" {
		errs["scope"] = "unknown scope " + bad
	} else if len(scopes) == 0 {
		errs["scope"] = "at least one scope"
	}
	// PKCE обязателен даже для конфиденциальных клиентов (OAuth 2.1).
	if p.CodeChallengeMethod != "S256" || len(p.CodeChallenge) != 43 || !pkceRe.MatchString(p.CodeChallenge) {
		errs["code_challenge"] = "PKCE with code_challenge_method=S256 is required"
	}
	if utf8.RuneCountInString(p.State) > 500 {
		errs["state"] = "too long"
	}
	if len(errs) > 0 {
		writeFieldErrors(w, http.StatusBadRequest, "invalid_request", "invalid authorization request", errs)
		return nil, nil, false
	}
	if viewerFrom(r.Context()).Bot != 0 {
		writeError(w, http.StatusForbidden, "forbidden", "bots cannot authorize apps")
		return nil, nil, false
	}
	return app, scopes, true
}

// handleAuthorizeInfo: GET — что показать на странице согласия.
func (s *Server) handleAuthorizeInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := authorizeParams{ResponseType: q.Get("response_type"), ClientID: q.Get("client_id"),
		RedirectURI: q.Get("redirect_uri"), Scope: q.Get("scope"), State: q.Get("state"),
		CodeChallenge: q.Get("code_challenge"), CodeChallengeMethod: q.Get("code_challenge_method")}
	app, scopes, ok := s.checkAuthorize(w, r, p)
	if !ok {
		return
	}
	owner, err := s.store.UserByID(r.Context(), app.OwnerID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	type scopeView struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	views := make([]scopeView, len(scopes))
	for i, sc := range scopes {
		views[i] = scopeView{sc, scopeInfo[sc]}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":          map[string]any{"name": app.Name, "client_id": app.ClientID, "developer": owner.Username},
		"scopes":       views,
		"redirect_uri": p.RedirectURI,
	})
}

func redirectWith(base string, params url.Values) string {
	u, _ := url.Parse(base) // уже проверен
	q := u.Query()
	for k, v := range params {
		q[k] = v
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// handleAuthorize: POST — пользователь нажал «Разрешить» или «Отмена».
// Ответ — адрес, куда фронтенд отправит браузер.
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	var p authorizeParams
	if !decodeJSON(w, r, &p) {
		return
	}
	app, scopes, ok := s.checkAuthorize(w, r, p)
	if !ok {
		return
	}
	params := url.Values{"iss": {s.cfg.AppURL}}
	if p.State != "" {
		params.Set("state", p.State)
	}
	if !p.Approve {
		params.Set("error", "access_denied")
		writeJSON(w, http.StatusOK, map[string]string{"redirect_to": redirectWith(p.RedirectURI, params)})
		return
	}
	me := userIDFrom(r.Context())
	code, hash := randomToken("", 32)
	if err := s.store.CreateAuthCode(r.Context(), hash, app.ID, me, scopes, p.RedirectURI, p.CodeChallenge, authCodeTTL); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.event(r, me, "app_authorized")
	params.Set("code", code)
	writeJSON(w, http.StatusOK, map[string]string{"redirect_to": redirectWith(p.RedirectURI, params)})
}

// ---------- токен-эндпоинт ----------

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="oauth"`)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// clientAuth: client_id + (для конфиденциальных) секрет из Basic или формы.
func (s *Server) clientAuth(w http.ResponseWriter, r *http.Request) (*store.App, bool) {
	clientID, secret, basic := r.BasicAuth()
	if basic {
		// RFC 6749 §2.3.1: значения в Basic закодированы как form-urlencoded
		clientID, _ = url.QueryUnescape(clientID)
		secret, _ = url.QueryUnescape(secret)
	} else {
		clientID, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	app, err := s.store.AppByClientID(r.Context(), clientID)
	if errors.Is(err, store.ErrNotFound) {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "unknown client")
		return nil, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	if app.Confidential {
		if secret == "" || subtle.ConstantTimeCompare(auth.HashRefreshToken(secret), app.SecretHash) != 1 {
			oauthError(w, http.StatusUnauthorized, "invalid_client", "client authentication failed")
			return nil, false
		}
	}
	return app, true
}

func parseForm(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		oauthError(w, http.StatusBadRequest, "invalid_request", "use application/x-www-form-urlencoded")
		return false
	}
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "malformed form")
		return false
	}
	return true
}

func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if gt := r.PostForm.Get("grant_type"); gt != "authorization_code" {
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	app, ok := s.clientAuth(w, r)
	if !ok {
		return
	}
	code, err := s.store.UseAuthCode(r.Context(), auth.HashRefreshToken(r.PostForm.Get("code")))
	if errors.Is(err, store.ErrCodeReused) {
		s.log.Warn("oauth code reuse, tokens revoked", "app_id", app.ID)
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code already used")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	sum := sha256.Sum256([]byte(verifier))
	switch {
	case code.AppID != app.ID:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code was issued to another client")
		return
	case r.PostForm.Get("redirect_uri") != code.RedirectURI:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	case !pkceRe.MatchString(verifier) ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(code.Challenge)) != 1:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	token, hash := randomToken(apiTokenPrefix, 32)
	var expires *time.Time
	if !slices.Contains(code.Scopes, "offline") {
		t := time.Now().Add(oauthTokenTTL)
		expires = &t
	}
	appID, codeID := app.ID, code.ID
	if _, err := s.store.CreateAPIToken(r.Context(), store.NewAPIToken{Hash: hash, Kind: "oauth", UserID: code.UserID,
		AppID: &appID, CodeID: &codeID, Scopes: code.Scopes, ExpiresAt: expires}); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Pragma", "no-cache")
	resp := map[string]any{"access_token": token, "token_type": "Bearer",
		"scope": strings.Join(code.Scopes, " "), "user_id": code.UserID}
	if expires != nil {
		resp["expires_in"] = int(oauthTokenTTL.Seconds())
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleOAuthRevoke — RFC 7009: на неизвестный токен тоже 200.
func (s *Server) handleOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	app, ok := s.clientAuth(w, r)
	if !ok {
		return
	}
	if err := s.store.RevokeAPIToken(r.Context(), auth.HashRefreshToken(r.PostForm.Get("token")), app.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ---------- «Приложения с доступом» в настройках ----------

func (s *Server) handleAuthorizedApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.AuthorizedApps(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": apps})
}

func (s *Server) handleRevokeAuthorizedApp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	me := userIDFrom(r.Context())
	found, err := s.store.RevokeAppForUser(r.Context(), me, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "app has no access")
		return
	}
	s.event(r, me, "app_revoked")
	w.WriteHeader(http.StatusNoContent)
}
