// Package yookassa — минимальный клиент API ЮKassa v3: создать платёж
// с чеком по 54-ФЗ и узнать его статус. Документация:
// https://yookassa.ru/developers/api
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Amount struct {
	Value    string `json:"value"` // "99.00"
	Currency string `json:"currency"`
}

// Kopecks → Amount в рублях. Деньги считаем в копейках, без float.
func RUB(kopecks int64) Amount {
	return Amount{Value: fmt.Sprintf("%d.%02d", kopecks/100, kopecks%100), Currency: "RUB"}
}

// ParseKopecks — обратное преобразование "99.00" → 9900.
func ParseKopecks(v string) (int64, error) {
	rub, kop, ok := strings.Cut(v, ".")
	if !ok {
		kop = "00"
	}
	if len(kop) == 1 {
		kop += "0"
	}
	if len(kop) != 2 {
		return 0, fmt.Errorf("bad amount %q", v)
	}
	r, err1 := strconv.ParseInt(rub, 10, 64)
	k, err2 := strconv.ParseInt(kop, 10, 64)
	if err1 != nil || err2 != nil || r < 0 || k < 0 {
		return 0, fmt.Errorf("bad amount %q", v)
	}
	return r*100 + k, nil
}

type ReceiptItem struct {
	Description    string `json:"description"`
	Quantity       string `json:"quantity"`
	Amount         Amount `json:"amount"`
	VATCode        int    `json:"vat_code"`
	PaymentMode    string `json:"payment_mode"`    // full_payment
	PaymentSubject string `json:"payment_subject"` // service
	Measure        string `json:"measure,omitempty"`
}

type Receipt struct {
	Customer struct {
		Email string `json:"email,omitempty"`
	} `json:"customer"`
	Items []ReceiptItem `json:"items"`
}

type Confirmation struct {
	Type            string `json:"type"`
	ReturnURL       string `json:"return_url,omitempty"`
	ConfirmationURL string `json:"confirmation_url,omitempty"`
}

type CreateRequest struct {
	Amount       Amount            `json:"amount"`
	Capture      bool              `json:"capture"`
	Confirmation Confirmation      `json:"confirmation"`
	Description  string            `json:"description"`
	Metadata     map[string]string `json:"metadata"`
	Receipt      *Receipt          `json:"receipt,omitempty"`
}

type Payment struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"` // pending | waiting_for_capture | succeeded | canceled
	Paid         bool              `json:"paid"`
	Amount       Amount            `json:"amount"`
	Confirmation *Confirmation     `json:"confirmation"`
	Metadata     map[string]string `json:"metadata"`
	Test         bool              `json:"test"`
}

type Client struct {
	BaseURL   string
	ShopID    string
	SecretKey string
	HTTP      *http.Client
}

func New(baseURL, shopID, secret string) *Client {
	return &Client{BaseURL: baseURL, ShopID: shopID, SecretKey: secret, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// APIError — ответ ЮKassa с ошибкой.
type APIError struct {
	Status      int
	Code        string `json:"code"`
	Description string `json:"description"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("yookassa %d %s: %s", e.Status, e.Code, e.Description)
}

func (c *Client) do(ctx context.Context, method, path, idemKey string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, r)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.ShopID, c.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	if idemKey != "" {
		// повтор с тем же ключом не создаст второй платёж
		req.Header.Set("Idempotence-Key", idemKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		e := &APIError{Status: resp.StatusCode}
		_ = json.Unmarshal(data, e)
		return e
	}
	return json.Unmarshal(data, out)
}

func (c *Client) CreatePayment(ctx context.Context, req CreateRequest, idemKey string) (*Payment, error) {
	p := &Payment{}
	return p, c.do(ctx, http.MethodPost, "/payments", idemKey, req, p)
}

// GetPayment — актуальный статус. Уведомлениям ЮKassa не подписаны,
// поэтому каждому уведомлению верим только после этого запроса.
func (c *Client) GetPayment(ctx context.Context, id string) (*Payment, error) {
	if id == "" || strings.ContainsAny(id, "/?#%") {
		return nil, errors.New("bad payment id")
	}
	p := &Payment{}
	return p, c.do(ctx, http.MethodGet, "/payments/"+id, "", nil, p)
}

// NotificationNets — адреса, с которых ЮKassa шлёт уведомления
// (https://yookassa.ru/developers/using-api/webhooks).
var NotificationNets = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"185.71.76.0/27", "185.71.77.0/27", "77.75.153.0/25", "77.75.156.11/32",
		"77.75.156.35/32", "77.75.154.128/25", "2a02:5180::/32"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

func FromYooKassa(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range NotificationNets {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Notification — тело уведомления: {"type":"notification","event":"payment.succeeded","object":{...}}.
type Notification struct {
	Type   string `json:"type"`
	Event  string `json:"event"`
	Object struct {
		ID string `json:"id"`
	} `json:"object"`
}
