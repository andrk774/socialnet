package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"socialnet/internal/av"
	"socialnet/internal/store"
)

const maxDocBytes = 50 << 20

// docFamilies: какие расширения разрешены и какой сигнатурой должен
// начинаться файл. Расширение без совпадающей сигнатуры — отказ: так под
// видом .pdf не загрузить исполняемый файл или HTML-страницу.
var docFamilies = map[string]string{
	"pdf":  "pdf",
	"docx": "zip", "xlsx": "zip", "pptx": "zip", "odt": "zip", "ods": "zip", "odp": "zip", "epub": "zip", "zip": "zip",
	"doc": "ole", "xls": "ole", "ppt": "ole",
	"7z": "7z", "rar": "rar", "gz": "gz", "rtf": "rtf", "djvu": "djvu",
	"txt": "text", "md": "text", "csv": "text",
}

var docMagic = map[string][][]byte{
	"pdf":  {[]byte("%PDF-")},
	"zip":  {[]byte("PK\x03\x04"), []byte("PK\x05\x06")},
	"ole":  {{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}},
	"7z":   {{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}},
	"rar":  {[]byte("Rar!\x1A\x07")},
	"gz":   {{0x1F, 0x8B}},
	"rtf":  {[]byte(`{\rtf`)},
	"djvu": {[]byte("AT&TFORM")},
}

// detectDocument возвращает расширение, если имя и содержимое согласны.
func detectDocument(filename string, head []byte) (string, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	family, ok := docFamilies[ext]
	if !ok {
		return "", false
	}
	if family == "text" {
		// текст: валидный UTF-8 без нулевых байтов (последний символ мог обрезаться)
		if bytes.IndexByte(head, 0) >= 0 {
			return "", false
		}
		for i := 0; i < 4 && len(head) > 0 && !utf8.Valid(head); i++ {
			head = head[:len(head)-1]
		}
		return ext, utf8.Valid(head)
	}
	for _, m := range docMagic[family] {
		if bytes.HasPrefix(head, m) {
			return ext, true
		}
	}
	return "", false
}

func docTitle(field, filename string) string {
	t := cleanText(field, 120)
	if t == "" {
		base := filepath.Base(strings.ReplaceAll(filename, `\`, "/"))
		t = cleanText(strings.TrimSuffix(base, filepath.Ext(base)), 120)
	}
	if t == "" {
		t = "Документ"
	}
	return t
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	me := userIDFrom(r.Context())
	if !s.uploadLimiter.allow(strconv.FormatInt(me, 10)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads, slow down")
		return
	}
	up, ok := s.receiveUpload(w, r, maxDocBytes)
	if !ok {
		return
	}
	defer up.cleanup()

	f, err := os.Open(up.Path)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	head := make([]byte, 64<<10)
	n, _ := io.ReadFull(f, head)
	_ = f.Close()
	ext, ok := detectDocument(up.Filename, head[:n])
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type",
			"supported: PDF, DOC(X), XLS(X), PPT(X), ODT/ODS/ODP, EPUB, DjVu, RTF, TXT, MD, CSV, ZIP, 7Z, RAR, GZ")
		return
	}
	if blocked, err := s.store.IsBlocked(r.Context(), up.SHA); err != nil {
		s.internalError(w, r, err)
		return
	} else if blocked {
		writeError(w, http.StatusUnprocessableEntity, "blocked_content", "this file was removed by moderators")
		return
	}

	id := av.NewID()
	dir, _ := s.avStore.Dir("docs", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.internalError(w, r, err)
		return
	}
	dst, _ := s.avStore.Path("docs", id, "file.bin")
	if err := os.Rename(up.Path, dst); err != nil {
		_ = s.avStore.Delete("docs", id)
		s.internalError(w, r, err)
		return
	}
	if err := s.avStore.Commit(r.Context(), "docs", id); err != nil {
		_ = s.avStore.Delete("docs", id)
		s.internalError(w, r, err)
		return
	}
	doc, err := s.store.CreateDocument(r.Context(), store.NewDocument{
		ID: id, Owner: me, Title: docTitle(up.Fields["title"], up.Filename), Ext: ext, Size: up.Size, SHA256: up.SHA,
	})
	if err != nil {
		_ = s.avStore.Delete("docs", id)
		s.internalError(w, r, err)
		return
	}
	s.m.business.Inc("document_uploaded")
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) handleMyDocuments(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	items, err := s.store.MyDocuments(r.Context(), userIDFrom(r.Context()), p.Before, p.Limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list(items, p.Limit, store.Document.Cursor))
}

func docID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(chi.URLParam(r, "did"))
	if !validUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return "", false
	}
	return id, true
}

func (s *Server) handleRenameDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := docID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	title := cleanText(req.Title, 120)
	if title == "" {
		writeFieldErrors(w, http.StatusUnprocessableEntity, "validation_failed", "invalid input",
			map[string]string{"title": "1-120 characters"})
		return
	}
	err := s.store.RenameDocument(r.Context(), id, userIDFrom(r.Context()), title)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	doc, err := s.store.Document(r.Context(), id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := docID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteDocument(r.Context(), id, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.avStore.Delete("docs", id); err != nil {
		s.log.Warn("delete document file", "err", err, "id", id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDownloadDocument: GET /documents/{did}/download — с проверкой доступа.
// Всегда как вложение и application/octet-stream: браузер не откроет файл
// на нашем домене, а значит HTML или SVG под видом документа не исполнятся.
func (s *Server) handleDownloadDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := docID(w, r)
	if !ok {
		return
	}
	doc, err := s.store.Document(r.Context(), id)
	if err == nil {
		var allowed bool
		if allowed, err = s.store.DocumentAccess(r.Context(), userIDFrom(r.Context()), id); err == nil && !allowed {
			err = store.ErrNotFound
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Disposition", contentDisposition(doc.Title+"."+doc.Ext))
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=3600")
	s.serveAVFile(w, r, "docs", id, "file.bin", "application/octet-stream")
}

// contentDisposition: ASCII-имя для старых клиентов и полное — по RFC 6266/5987.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' || r == '/' || r == ';' {
			return '_'
		}
		return r
	}, name)
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + url.PathEscape(name)
}
