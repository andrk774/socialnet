// Package s3 — клиент S3-совместимых хранилищ (MinIO, Yandex Object Storage,
// VK Cloud, AWS) с подписью AWS Signature V4. Только нужные операции:
// PutObject, GetObject (с Range), DeleteObject, ListObjectsV2, presign.
package s3

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("s3: object not found")
	ErrRange    = errors.New("s3: range not satisfiable")
)

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type Client struct {
	Endpoint  string // https://storage.yandexcloud.net или http://minio:9000
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool // MinIO: endpoint/bucket/key; иначе bucket.endpoint/key
	HTTP      *http.Client
	now       func() time.Time
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now().UTC()
}

// objectURL строит адрес объекта; ключ кодируется по правилам S3 (слэши сохраняются).
func (c *Client) objectURL(key string) (*url.URL, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" {
		return nil, errors.New("s3: bad endpoint")
	}
	if c.PathStyle {
		u.Path = "/" + c.Bucket + "/" + key
	} else {
		u.Host = c.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	u.RawPath = escapePath(u.Path)
	return u, nil
}

// escapePath — URI-encode каждого сегмента (RFC 3986 unreserved не трогаем).
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = uriEncode(s, true)
	}
	return strings.Join(segs, "/")
}

func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z') || ('0' <= ch && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' || ch == '~' || (ch == '/' && !encodeSlash) {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

func (c *Client) signingKey(date string) []byte {
	k := hmacSHA256([]byte("AWS4"+c.SecretKey), date)
	k = hmacSHA256(k, c.Region)
	k = hmacSHA256(k, "s3")
	return hmacSHA256(k, "aws4_request")
}

// signature считает подпись запроса. headers — то, что подписываем (в нижнем регистре).
func (c *Client) signature(method string, u *url.URL, query url.Values, headers map[string]string, payloadHash string, t time.Time) (sig, signed string) {
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + strings.Join(strings.Fields(headers[k]), " ") + "\n")
	}
	signed = strings.Join(names, ";")
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{method, path, canonicalQuery(query), ch.String(), signed, payloadHash}, "\n")
	date := t.Format("20060102")
	scope := date + "/" + c.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + t.Format("20060102T150405Z") + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	return hex.EncodeToString(hmacSHA256(c.signingKey(date), toSign)), signed
}

// sign добавляет к запросу заголовки x-amz-* и Authorization.
// Подписываются host, x-amz-* и переданные extra (например, range).
func (c *Client) sign(req *http.Request, payloadHash string, extra ...string) {
	t := c.clock()
	req.Header.Set("x-amz-date", t.Format("20060102T150405Z"))
	req.Header.Set("x-amz-content-sha256", payloadHash)
	headers := map[string]string{"host": req.URL.Host,
		"x-amz-date": req.Header.Get("x-amz-date"), "x-amz-content-sha256": payloadHash}
	for _, h := range extra {
		if v := req.Header.Get(h); v != "" {
			headers[strings.ToLower(h)] = v
		}
	}
	sig, signed := c.signature(req.Method, req.URL, req.URL.Query(), headers, payloadHash, t)
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s/%s/s3/aws4_request, SignedHeaders=%s, Signature=%s",
		c.AccessKey, t.Format("20060102"), c.Region, signed, sig))
}

// Presign — ссылка на объект без заголовков, действует expires.
func (c *Client) Presign(method, key string, expires time.Duration) (string, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return "", err
	}
	t := c.clock()
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", fmt.Sprintf("%s/%s/%s/s3/aws4_request", c.AccessKey, t.Format("20060102"), c.Region))
	q.Set("X-Amz-Date", t.Format("20060102T150405Z"))
	q.Set("X-Amz-Expires", strconv.Itoa(int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	sig, _ := c.signature(method, u, q, map[string]string{"host": u.Host}, "UNSIGNED-PAYLOAD", t)
	u.RawQuery = canonicalQuery(q) + "&X-Amz-Signature=" + sig
	return u.String(), nil
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func apiError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	_ = xml.Unmarshal(b, &e)
	return fmt.Errorf("s3: %d %s %s", resp.StatusCode, e.Code, e.Message)
}

func (c *Client) Put(ctx context.Context, key string, body []byte, contentType string) error {
	u, err := c.objectURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", contentType)
	c.sign(req, sha256Hex(body), "content-type")
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return nil
}

type Object struct {
	Body          io.ReadCloser
	Status        int // 200 или 206
	ContentLength int64
	ContentRange  string
	ContentType   string
	ETag          string
	LastModified  time.Time
}

// Get читает объект; rangeHeader («bytes=0-99») передаётся как есть.
func (c *Client) Get(ctx context.Context, key, rangeHeader string) (*Object, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	c.sign(req, emptySHA256, "range")
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, ErrNotFound
	case http.StatusRequestedRangeNotSatisfiable:
		_ = resp.Body.Close()
		return nil, ErrRange
	default:
		defer func() { _ = resp.Body.Close() }()
		return nil, apiError(resp)
	}
	mod, _ := http.ParseTime(resp.Header.Get("Last-Modified"))
	return &Object{Body: resp.Body, Status: resp.StatusCode, ContentLength: resp.ContentLength,
		ContentRange: resp.Header.Get("Content-Range"), ContentType: resp.Header.Get("Content-Type"),
		ETag: resp.Header.Get("ETag"), LastModified: mod}, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	u, err := c.objectURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	c.sign(req, emptySHA256)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return apiError(resp)
	}
	return nil
}

// List — ключи с префиксом (ListObjectsV2, все страницы).
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		u, err := c.objectURL("")
		if err != nil {
			return nil, err
		}
		if !c.PathStyle {
			u.Path, u.RawPath = "/", ""
		} else {
			u.Path, u.RawPath = "/"+c.Bucket, ""
		}
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		u.RawQuery = canonicalQuery(q)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		c.sign(req, emptySHA256)
		resp, err := c.http().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			err := apiError(resp)
			_ = resp.Body.Close()
			return nil, err
		}
		var out struct {
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, o := range out.Contents {
			keys = append(keys, o.Key)
		}
		if !out.IsTruncated || out.NextContinuationToken == "" {
			return keys, nil
		}
		token = out.NextContinuationToken
	}
}

// Verify проверяет подпись входящего запроса (для тестового сервера S3).
func (c *Client) Verify(r *http.Request, body []byte) error {
	auth := r.Header.Get("Authorization")
	_, rest, ok := strings.Cut(auth, "SignedHeaders=")
	if !ok || !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential="+c.AccessKey+"/") {
		return errors.New("bad authorization header")
	}
	signedList, sigPart, _ := strings.Cut(rest, ", Signature=")
	t, err := time.Parse("20060102T150405Z", r.Header.Get("x-amz-date"))
	if err != nil {
		return errors.New("bad x-amz-date")
	}
	payload := r.Header.Get("x-amz-content-sha256")
	if body != nil && payload != sha256Hex(body) {
		return errors.New("payload hash mismatch")
	}
	headers := map[string]string{}
	for _, h := range strings.Split(signedList, ";") {
		if h == "host" {
			headers[h] = r.Host
		} else {
			headers[h] = r.Header.Get(h)
		}
	}
	u := *r.URL
	sig, _ := c.signature(r.Method, &u, r.URL.Query(), headers, payload, t)
	if !hmac.Equal([]byte(sig), []byte(sigPart)) {
		return errors.New("signature mismatch")
	}
	return nil
}

// EnsureBucket создаёт бакет, если его нет (удобно для локального SeaweedFS/MinIO).
// В облаке у ключа может не быть права на создание — тогда ошибка не фатальна.
func (c *Client) EnsureBucket(ctx context.Context) error {
	u, err := c.objectURL("")
	if err != nil {
		return err
	}
	if c.PathStyle {
		u.Path, u.RawPath = "/"+c.Bucket, ""
	} else {
		u.Path, u.RawPath = "/", ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), nil)
	if err != nil {
		return err
	}
	c.sign(req, emptySHA256)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict { // 409: уже есть
		return nil
	}
	return apiError(resp)
}
