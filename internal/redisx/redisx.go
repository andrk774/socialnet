// Package redisx — небольшой клиент Redis (протокол RESP2): пул соединений,
// команды и pub/sub. Нам нужны лишь несколько команд, поэтому своя
// реализация на ~300 строк вместо внешней зависимости.
package redisx

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNil — ответ nil (ключа нет).
var ErrNil = errors.New("redis: nil")

// Error — ошибка, которую вернул сам Redis (-ERR ...).
type Error string

func (e Error) Error() string { return "redis: " + string(e) }

type Options struct {
	Addr     string
	Username string
	Password string
	DB       int
	TLS      bool
	PoolSize int
}

// ParseURL: redis://[:password@]host:port/db или rediss:// для TLS.
func ParseURL(raw string) (Options, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Host == "" {
		return Options{}, errors.New("REDIS_URL must look like redis://[:password@]host:6379/0")
	}
	o := Options{Addr: u.Host, TLS: u.Scheme == "rediss", PoolSize: 20}
	if u.Port() == "" {
		o.Addr = net.JoinHostPort(u.Hostname(), "6379")
	}
	if u.User != nil {
		o.Username = u.User.Username()
		o.Password, _ = u.User.Password()
		if _, has := u.User.Password(); !has { // redis://password@host
			o.Password, o.Username = o.Username, ""
		}
	}
	if db := strings.Trim(u.Path, "/"); db != "" {
		if o.DB, err = strconv.Atoi(db); err != nil {
			return o, errors.New("bad redis db number")
		}
	}
	return o, nil
}

type conn struct {
	c  net.Conn
	rd *bufio.Reader
	wr *bufio.Writer
}

type Client struct {
	opt  Options
	pool chan *conn
}

func New(opt Options) *Client {
	if opt.PoolSize <= 0 {
		opt.PoolSize = 10
	}
	return &Client{opt: opt, pool: make(chan *conn, opt.PoolSize)}
}

func (c *Client) dial(ctx context.Context) (*conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var nc net.Conn
	var err error
	if c.opt.TLS {
		host, _, _ := net.SplitHostPort(c.opt.Addr)
		td := tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
		nc, err = td.DialContext(ctx, "tcp", c.opt.Addr)
	} else {
		nc, err = d.DialContext(ctx, "tcp", c.opt.Addr)
	}
	if err != nil {
		return nil, err
	}
	cn := &conn{c: nc, rd: bufio.NewReader(nc), wr: bufio.NewWriter(nc)}
	init := [][]any{}
	if c.opt.Password != "" {
		if c.opt.Username != "" {
			init = append(init, []any{"AUTH", c.opt.Username, c.opt.Password})
		} else {
			init = append(init, []any{"AUTH", c.opt.Password})
		}
	}
	if c.opt.DB != 0 {
		init = append(init, []any{"SELECT", c.opt.DB})
	}
	for _, args := range init {
		if _, err := cn.roundTrip(ctx, args); err != nil {
			_ = nc.Close()
			return nil, err
		}
	}
	return cn, nil
}

func (c *Client) get(ctx context.Context) (*conn, error) {
	select {
	case cn := <-c.pool:
		return cn, nil
	default:
		return c.dial(ctx)
	}
}

func (c *Client) put(cn *conn, broken bool) {
	if broken {
		_ = cn.c.Close()
		return
	}
	select {
	case c.pool <- cn:
	default:
		_ = cn.c.Close()
	}
}

func (c *Client) Close() {
	for {
		select {
		case cn := <-c.pool:
			_ = cn.c.Close()
		default:
			return
		}
	}
}

// Do выполняет команду. Ответ: string, int64, []any, nil или ошибка.
func (c *Client) Do(ctx context.Context, args ...any) (any, error) {
	cn, err := c.get(ctx)
	if err != nil {
		return nil, err
	}
	v, err := cn.roundTrip(ctx, args)
	var redisErr Error
	c.put(cn, err != nil && !errors.As(err, &redisErr))
	return v, err
}

func (cn *conn) roundTrip(ctx context.Context, args []any) (any, error) {
	dl, ok := ctx.Deadline()
	if !ok {
		dl = time.Now().Add(5 * time.Second)
	}
	_ = cn.c.SetDeadline(dl)
	if err := writeCommand(cn.wr, args); err != nil {
		return nil, err
	}
	if err := cn.wr.Flush(); err != nil {
		return nil, err
	}
	return readReply(cn.rd)
}

func writeCommand(w *bufio.Writer, args []any) error {
	_, _ = fmt.Fprintf(w, "*%d\r\n", len(args))
	for _, a := range args {
		var s string
		switch v := a.(type) {
		case string:
			s = v
		case []byte:
			s = string(v)
		case int:
			s = strconv.Itoa(v)
		case int64:
			s = strconv.FormatInt(v, 10)
		case float64:
			s = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return fmt.Errorf("redis: unsupported argument type %T", a)
		}
		_, _ = fmt.Fprintf(w, "$%d\r\n%s\r\n", len(s), s)
	}
	return nil
}

const maxBulk = 64 << 20

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", errors.New("redis: bad line ending")
	}
	return line[:len(line)-2], nil
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if line == "" {
		return nil, errors.New("redis: empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, Error(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil || n > maxBulk {
			return nil, errors.New("redis: bad bulk length")
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil || n > 1<<20 {
			return nil, errors.New("redis: bad array length")
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = readReply(r); err != nil {
				var re Error
				if !errors.As(err, &re) {
					return nil, err
				}
				out[i] = err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("redis: unknown reply type %q", line[0])
}

// ---------- удобные обёртки ----------

func (c *Client) String(ctx context.Context, args ...any) (string, error) {
	v, err := c.Do(ctx, args...)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", ErrNil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("redis: unexpected %T", v)
	}
	return s, nil
}

func (c *Client) Int(ctx context.Context, args ...any) (int64, error) {
	v, err := c.Do(ctx, args...)
	if err != nil {
		return 0, err
	}
	switch x := v.(type) {
	case int64:
		return x, nil
	case nil:
		return 0, ErrNil
	}
	return 0, fmt.Errorf("redis: unexpected %T", v)
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Do(ctx, "PING")
	return err
}

// ---------- pub/sub ----------

type Message struct {
	Channel string
	Payload string
}

// Subscribe держит отдельное соединение и при обрыве переподключается сам.
// Сообщения идут в канал до отмены ctx.
func (c *Client) Subscribe(ctx context.Context, channels ...string) <-chan Message {
	out := make(chan Message, 256)
	go func() {
		defer close(out)
		backoff := 100 * time.Millisecond
		for ctx.Err() == nil {
			err := c.subscribeOnce(ctx, channels, out, func() { backoff = 100 * time.Millisecond })
			if ctx.Err() != nil {
				return
			}
			_ = err // соединение оборвалось — подождём и переподключимся
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
		}
	}()
	return out
}

func (c *Client) subscribeOnce(ctx context.Context, channels []string, out chan<- Message, ok func()) error {
	cn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	var once sync.Once
	stop := context.AfterFunc(ctx, func() { once.Do(func() { _ = cn.c.Close() }) })
	defer stop()
	defer once.Do(func() { _ = cn.c.Close() })

	args := []any{"SUBSCRIBE"}
	for _, ch := range channels {
		args = append(args, ch)
	}
	_ = cn.c.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeCommand(cn.wr, args); err != nil {
		return err
	}
	if err := cn.wr.Flush(); err != nil {
		return err
	}
	_ = cn.c.SetDeadline(time.Time{})
	for {
		v, err := readReply(cn.rd)
		if err != nil {
			return err
		}
		arr, _ := v.([]any)
		if len(arr) == 3 {
			if kind, _ := arr[0].(string); kind == "message" {
				ch, _ := arr[1].(string)
				p, _ := arr[2].(string)
				select {
				case out <- Message{Channel: ch, Payload: p}:
				default: // потребитель не успевает — событие теряется, как и в самом Redis pub/sub
				}
			} else if kind == "subscribe" {
				ok()
			}
		}
	}
}
