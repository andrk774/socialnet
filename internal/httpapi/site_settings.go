package httpapi

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type siteSettings struct {
	PublicURL     string `json:"public_url"`
	IngressClass  string `json:"ingress_class"`
	TLSSecretName string `json:"tls_secret"`
	CookieSecure  bool   `json:"cookie_secure"`
}

func (s *Server) loadSite(ctx context.Context) (siteSettings, error) {
	// #nosec G101 -- TLSSecretName is a Kubernetes resource name, not a credential.
	c := siteSettings{PublicURL: s.cfg.AppURL, IngressClass: "nginx", TLSSecretName: "socialnet-tls", CookieSecure: s.cfg.CookieSecure}
	err := s.store.Pool().QueryRow(ctx, `SELECT public_url,ingress_class,tls_secret FROM site_settings WHERE id=true`).Scan(&c.PublicURL, &c.IngressClass, &c.TLSSecretName)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return c, err
}
func (s *Server) publicURL(ctx context.Context) string {
	c, e := s.loadSite(ctx)
	if e != nil {
		return s.cfg.AppURL
	}
	return c.PublicURL
}
func (s *Server) handleGetSite(w http.ResponseWriter, r *http.Request) {
	c, e := s.loadSite(r.Context())
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, c)
}

var siteName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func (s *Server) handlePutSite(w http.ResponseWriter, r *http.Request) {
	var c siteSettings
	if !decodeJSON(w, r, &c) {
		return
	}
	c.PublicURL = strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
	u, e := url.Parse(c.PublicURL)
	if e != nil || len(c.PublicURL) > 512 || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") || len(c.IngressClass) > 253 || len(c.TLSSecretName) > 253 || !siteName.MatchString(c.IngressClass) || !siteName.MatchString(c.TLSSecretName) {
		writeError(w, 422, "validation", "invalid site URL or Kubernetes resource name")
		return
	}
	_, e = s.store.Pool().Exec(r.Context(), `INSERT INTO site_settings(id,public_url,ingress_class,tls_secret) VALUES(true,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET public_url=$1,ingress_class=$2,tls_secret=$3`, c.PublicURL, c.IngressClass, c.TLSSecretName)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	w.WriteHeader(204)
}
