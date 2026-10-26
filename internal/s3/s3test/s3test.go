// Package s3test — S3-сервер в памяти для тестов: проверяет подпись
// каждого запроса, поддерживает PUT, GET с Range, DELETE и ListObjectsV2.
package s3test

import (
	"bytes"
	"encoding/xml"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"socialnet/internal/s3"
)

type object struct {
	data  []byte
	ctype string
	mod   time.Time
}

type Server struct {
	*httptest.Server
	mu      sync.Mutex
	objects map[string]object
	Client  *s3.Client // настроенный на этот сервер
	BadSigs int
}

func New(t *testing.T) *Server {
	s := &Server{objects: map[string]object{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	s.Client = &s3.Client{Endpoint: s.URL, Region: "ru-central1", Bucket: "media", PathStyle: true,
		AccessKey: "test-access", SecretKey: "test-secret"}
	return s
}

func (s *Server) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var check []byte
	if r.Method == http.MethodPut {
		check = body
	}
	if err := s.Client.Verify(r, check); err != nil {
		s.mu.Lock()
		s.BadSigs++
		s.mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "<Error><Code>SignatureDoesNotMatch</Code><Message>"+html.EscapeString(err.Error())+"</Message></Error>")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/media")
	key := strings.TrimPrefix(path, "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		prefix := r.URL.Query().Get("prefix")
		type c struct {
			Key string `xml:"Key"`
		}
		var out struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []c      `xml:"Contents"`
		}
		for k := range s.objects {
			if strings.HasPrefix(k, prefix) {
				out.Contents = append(out.Contents, c{k})
			}
		}
		_ = xml.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPut && key == "": // CreateBucket
	case r.Method == http.MethodPut:
		s.objects[key] = object{data: body, ctype: r.Header.Get("Content-Type"), mod: time.Now().UTC().Truncate(time.Second)}
	case r.Method == http.MethodGet:
		o, ok := s.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		w.Header().Set("Content-Type", o.ctype)
		w.Header().Set("ETag", `"etag-`+key+`"`)
		http.ServeContent(w, r, "", o.mod, bytes.NewReader(o.data))
	case r.Method == http.MethodDelete:
		delete(s.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
