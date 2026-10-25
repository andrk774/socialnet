package httpapi

import (
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

type uploaded struct {
	Path     string // временный файл; удалить после обработки
	SHA      []byte
	Size     int64
	Fields   map[string]string
	Filename string
}

func (u *uploaded) cleanup() { _ = os.Remove(u.Path) }

// receiveUpload принимает multipart-форму потоком прямо на диск, считая SHA-256.
// Файл в поле "file", до maxBytes; текстовые поля — до 4 КБ каждое.
func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request, maxBytes int64) (*uploaded, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(64<<10))
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "multipart/form-data expected")
		return nil, false
	}
	tmpDir, err := s.avStore.TempDir()
	if err != nil {
		s.internalError(w, r, err)
		return nil, false
	}
	up := &uploaded{Fields: map[string]string{}}
	fail := func(status int, code, msg string) (*uploaded, bool) {
		if up.Path != "" {
			up.cleanup()
		}
		writeError(w, status, code, msg)
		return nil, false
	}

	for n := 0; ; n++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fail(http.StatusRequestEntityTooLarge, "too_large", "file is too large")
		}
		if err != nil || n > 20 {
			return fail(http.StatusBadRequest, "bad_request", "malformed form")
		}
		name := part.FormName()
		if name == "file" && up.Path == "" {
			f, err := os.CreateTemp(tmpDir, "upload-*")
			if err != nil {
				s.internalError(w, r, err)
				return nil, false
			}
			up.Path, up.Filename = f.Name(), part.FileName()
			h := sha256.New()
			written, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(part, maxBytes+1))
			if cerr := f.Close(); err == nil {
				err = cerr // ошибка записи на диск всплывает при закрытии
			}
			if errors.As(err, &maxErr) || written > maxBytes {
				return fail(http.StatusRequestEntityTooLarge, "too_large", "file is too large")
			}
			if err != nil {
				return fail(http.StatusBadRequest, "bad_request", "upload interrupted")
			}
			up.SHA, up.Size = h.Sum(nil), written
			continue
		}
		b, err := io.ReadAll(io.LimitReader(part, 4096))
		if err != nil {
			return fail(http.StatusBadRequest, "bad_request", "malformed form")
		}
		up.Fields[name] = string(b)
	}
	if up.Path == "" || up.Size == 0 {
		return fail(http.StatusBadRequest, "bad_request", `multipart field "file" is required`)
	}
	return up, true
}

// cleanText: обрезка, без управляющих символов (в том числе переводов строк
// и символов смены направления текста), не длиннее max символов.
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Bidi_Control, r) {
			return -1 // символы смены направления — приём для подделки имён файлов
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	s = strings.Join(strings.Fields(s), " ")
	for utf8.RuneCountInString(s) > max {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return strings.TrimSpace(s) // обрезка могла прийтись на пробел (нашёл фаззер)
}

// acquireAV ограничивает число одновременных запусков ffmpeg.
func (s *Server) acquireAV(w http.ResponseWriter) (func(), bool) {
	select {
	case s.avSem <- struct{}{}:
		return func() { <-s.avSem }, true
	default:
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusServiceUnavailable, "busy", "media processing is busy, try again in a few seconds")
		return nil, false
	}
}
