package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"socialnet/internal/media"
	"socialnet/internal/safehttp"
	"strings"
	"time"
	"unicode/utf8"
)

type aiSettings struct {
	Enabled    bool   `json:"enabled"`
	AllowUsers bool   `json:"allow_users"`
	BaseURL    string `json:"base_url"`
	TextModel  string `json:"text_model"`
	ImageModel string `json:"image_model"`
	APIKey     string `json:"api_key,omitempty"`
	KeySet     bool   `json:"key_set"`
	ClearKey   bool   `json:"clear_key,omitempty"`
}

func (s *Server) loadAI(ctx context.Context) (aiSettings, error) {
	c := aiSettings{BaseURL: "https://api.openai.com/v1"}
	var raw, enc []byte
	e := s.store.Pool().QueryRow(ctx, `SELECT settings,key_enc FROM ai_settings WHERE id=true`).Scan(&raw, &enc)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, e
	}
	key, e := s.box.Open(enc)
	c.APIKey = string(key)
	c.KeySet = len(key) > 0
	return c, e
}
func validAIURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func (s *Server) handleGetAI(w http.ResponseWriter, r *http.Request) {
	c, e := s.loadAI(r.Context())
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	c.APIKey = ""
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, c)
}
func (s *Server) handlePutAI(w http.ResponseWriter, r *http.Request) {
	var c aiSettings
	if !decodeJSON(w, r, &c) {
		return
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if !validAIURL(c.BaseURL) || len(c.BaseURL) > 512 || len(c.APIKey) > 4096 || len(c.TextModel) > 128 || len(c.ImageModel) > 128 || strings.ContainsAny(c.APIKey, "\r\n") {
		writeError(w, 422, "validation", "invalid AI configuration")
		return
	}
	old, e := s.loadAI(r.Context())
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	if c.ClearKey {
		c.APIKey = ""
	} else if c.APIKey == "" {
		c.APIKey = old.APIKey
	}
	if c.Enabled && (c.APIKey == "" || (c.TextModel == "" && c.ImageModel == "")) {
		writeError(w, 422, "validation", "API key and at least one model required")
		return
	}
	enc, e := s.box.Seal([]byte(c.APIKey))
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	c.APIKey = ""
	c.ClearKey = false
	c.KeySet = false
	raw, _ := json.Marshal(map[string]any{"enabled": c.Enabled, "allow_users": c.AllowUsers, "base_url": c.BaseURL, "text_model": c.TextModel, "image_model": c.ImageModel})
	_, e = s.store.Pool().Exec(r.Context(), `INSERT INTO ai_settings VALUES(true,$1,$2) ON CONFLICT(id) DO UPDATE SET settings=EXCLUDED.settings,key_enc=EXCLUDED.key_enc`, raw, enc)
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	c, e := s.loadAI(r.Context())
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	allowed := c.Enabled && (c.AllowUsers || viewerFrom(r.Context()).Role == "admin")
	writeJSON(w, 200, map[string]any{"text": allowed && c.TextModel != "", "images": allowed && c.ImageModel != ""})
}
func aiRequest(ctx context.Context, c aiSettings, path string, body any, out any) error {
	raw, _ := json.Marshal(body)
	req, e := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := safehttp.Client(150 * time.Second)
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("provider unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return fmt.Errorf("provider HTTP %d", res.StatusCode)
	}
	raw, e = io.ReadAll(io.LimitReader(res.Body, 16*1024*1024+1))
	if e != nil || len(raw) > 16*1024*1024 {
		return fmt.Errorf("provider response too large")
	}
	return json.Unmarshal(raw, out)
}
func (s *Server) handleGenerateAI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string `json:"kind"`
		Prompt string `json:"prompt"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if (req.Kind != "text" && req.Kind != "image") || req.Prompt == "" || utf8.RuneCountInString(req.Prompt) > 4000 {
		writeError(w, 422, "validation", "prompt 1-4000 characters and kind text/image required")
		return
	}
	c, e := s.loadAI(r.Context())
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	if !c.Enabled || (!c.AllowUsers && viewerFrom(r.Context()).Role != "admin") {
		writeError(w, 403, "disabled", "AI is disabled or restricted to administrators")
		return
	}
	if (req.Kind == "text" && c.TextModel == "") || (req.Kind == "image" && c.ImageModel == "") {
		writeError(w, 422, "validation", "model not configured")
		return
	}
	limit := 30
	if req.Kind == "image" {
		limit = 5
	}
	var calls int
	e = s.store.Pool().QueryRow(r.Context(), `INSERT INTO ai_usage VALUES($1,CURRENT_DATE,$2,1) ON CONFLICT(user_id,day,kind) DO UPDATE SET calls=ai_usage.calls+1 WHERE ai_usage.calls<$3 RETURNING calls`, userIDFrom(r.Context()), req.Kind, limit).Scan(&calls)
	if errors.Is(e, pgx.ErrNoRows) {
		writeError(w, 429, "quota", "daily AI request limit reached")
		return
	}
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	if req.Kind == "text" {
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		e = aiRequest(r.Context(), c, "/chat/completions", map[string]any{"model": c.TextModel, "messages": []map[string]string{{"role": "system", "content": "Improve the user's message: correct spelling and clarity, preserve meaning and language. Return only the revised text. Do not send it."}, {"role": "user", "content": req.Prompt}}, "max_completion_tokens": 1200}, &out)
		if e != nil || len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
			writeError(w, 502, "provider_error", "AI did not return text; check provider, model and quota")
			return
		}
		writeJSON(w, 200, map[string]string{"text": out.Choices[0].Message.Content})
		return
	}
	var out struct {
		Data []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
	}
	body := map[string]any{"model": c.ImageModel, "prompt": "Create one expressive chat sticker, centered, no text. User description: " + req.Prompt, "n": 1, "size": "1024x1024"}
	if !strings.HasPrefix(c.ImageModel, "gpt-image") {
		body["response_format"] = "b64_json"
	}
	e = aiRequest(r.Context(), c, "/images/generations", body, &out)
	if e != nil || len(out.Data) == 0 {
		writeError(w, 502, "provider_error", "AI did not return an image; check provider and model")
		return
	}
	data, e := base64.StdEncoding.DecodeString(out.Data[0].Base64)
	if e != nil || len(data) == 0 {
		writeError(w, 502, "provider_error", "provider must return b64_json image")
		return
	}
	image, e := media.Process(bytes.NewReader(data))
	if e != nil {
		writeError(w, 502, "provider_error", "invalid generated image")
		return
	}
	id, e := s.store.CreateMedia(r.Context(), userIDFrom(r.Context()), image.Mime, image.Width, image.Height, int64(len(image.Full)))
	if e != nil {
		s.internalError(w, r, e)
		return
	}
	if e = s.saveFiles(id, image); e != nil {
		_ = s.store.DeleteMedia(r.Context(), id, userIDFrom(r.Context()))
		s.internalError(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]string{"id": id, "url": "/media/" + id + "/full", "thumb_url": "/media/" + id + "/thumb"})
}
