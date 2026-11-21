package httpapi

import "net/http"

// This cookie is accepted only by the read-only photo route, never by API mutations.
func (s *Server) setMediaCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "media_access", Value: token, Path: "/media/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}

func (s *Server) mediaAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Add("Vary", "Cookie")
		w.Header().Add("Vary", "Authorization")
		if r.Header.Get("Authorization") == "" {
			if c, err := r.Cookie("media_access"); err == nil {
				r = r.Clone(r.Context())
				r.Header.Set("Authorization", "Bearer "+c.Value)
			}
		}
		s.requireAuth(next).ServeHTTP(w, r)
	})
}
