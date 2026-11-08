// Package fetch 是安全受限的 HTTP 抓取层：只发 GET，只允许 http/https，
// 对内网地址（回环 / 私网 / 链路本地 / 云元数据 / IPv6 唯一本地）一律阻断。
//
// 安全模型分三道（见 docs/adr/0004-online-book-source.md §3）：
//  1. 请求前校验 URL（协议 / 端口 / userinfo），并对 host 做 DNS 全量解析、逐个可用 IP 校验
//     （未指定地址 :: / 0.0.0.0 属 DNS 噪声，跳过不算命中；跳过见 ssrf.go）；
//  2. 建连时用 net.Dialer.Control 对真实 socket 地址再校验一次（防 DNS rebinding）；
//  3. 重定向的每一跳重新走 1，限制跳数，禁止 https→http 降级。
//
// 本包不做 HTML/JSON 解析、不做重试，也不接触业务语义。
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// 错误分类：调用方用 errors.Is 判断，文案只给人看。
var (
	// ErrBlocked 表示请求被安全策略拒绝（协议 / 内网地址 / 重定向违规）。
	ErrBlocked = errors.New("fetch: 已被安全策略拒绝")
	// ErrTooLarge 表示响应体超过上限（此时仍会返回被截断的 Response）。
	ErrTooLarge = errors.New("fetch: 响应体超过上限")
	// ErrTimeout 表示连接或读取超时。
	ErrTimeout = errors.New("fetch: 超时")
	// ErrStatus 表示服务端返回非 2xx 状态码（此时仍会返回 Response）。
	ErrStatus = errors.New("fetch: 非 2xx 响应")
	// ErrNetwork 表示其它网络层错误（DNS 失败、连接被拒等）。
	ErrNetwork = errors.New("fetch: 网络错误")
)

// Resolver 是 DNS 解析的最小接口，便于测试注入；net.DefaultResolver 满足它。
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Options 控制抓取行为；零值可用（New 会补齐默认值）。
type Options struct {
	// TotalTimeout 是单次请求的总超时（默认 30s）。
	TotalTimeout time.Duration
	// ConnectTimeout 是建立连接的超时（默认 5s）。
	ConnectTimeout time.Duration
	// HeaderTimeout 是等待响应头的超时，即「首字节」（默认 10s）。
	HeaderTimeout time.Duration
	// MaxBodyBytes 是响应体上限（默认 8 MB），超出即截断并标记 Truncated。
	MaxBodyBytes int64
	// MaxHeaderBytes 是响应头上限（默认 64 KB）。
	MaxHeaderBytes int64
	// MaxRedirects 是允许的最大重定向跳数（默认 5）。
	MaxRedirects int
	// QPS 是单源每秒请求数上限（默认 1）；<=0 表示不限速（仅测试用）。
	QPS float64
	// MaxConcurrent 是并发请求上限（默认 4）。
	MaxConcurrent int
	// UserAgent 是请求头 UA（默认 norvels-fetch）。
	UserAgent string
	// Headers 是每个请求附带的静态请求头（来自书源配置）。
	// 不安全键值在 New 时丢弃，见 headers.go；Host/Content-Length 等键不接受。
	Headers map[string]string
	// AllowedPorts 是允许访问的端口（默认 80、443）；为空时回落默认值。
	AllowedPorts []int
	// Resolver 是 DNS 解析器（默认 net.DefaultResolver）。
	Resolver Resolver
	// DialContext 覆盖默认拨号器（默认带 SSRF Control 校验）。
	// 测试用它把公网 IP 路由到本地 httptest；生产不要设置。
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// DefaultOptions 返回推荐默认值。
func DefaultOptions() Options {
	return Options{
		TotalTimeout:   30 * time.Second,
		ConnectTimeout: 5 * time.Second,
		HeaderTimeout:  10 * time.Second,
		MaxBodyBytes:   8 << 20,
		MaxHeaderBytes: 64 << 10,
		MaxRedirects:   5,
		QPS:            1,
		MaxConcurrent:  4,
		UserAgent:      "norvels-fetch",
		AllowedPorts:   []int{80, 443},
	}
}

// Response 是一次抓取的结果；截断与错误都会带上它，便于排障。
type Response struct {
	Status    int         // HTTP 状态码
	Headers   http.Header // 响应头副本
	Body      []byte      // 响应体（可能被截断）
	FinalURL  string      // 重定向后的最终 URL
	Truncated bool        // 是否因超过 MaxBodyBytes 被截断
}

// Client 是一个按来源配置的抓取器（并发安全）。
type Client struct {
	opt      Options
	resolver Resolver
	limiter  *limiter
	sem      chan struct{}
	hc       *http.Client
	headers  http.Header // 只含安全项的静态请求头（New 时清洗一次）
}

// New 按 opts 构造抓取器；opts 的零值字段用默认值补齐。
func New(opts Options) *Client {
	opt := withDefaults(opts)
	c := &Client{
		opt:      opt,
		resolver: opt.Resolver,
		limiter:  newLimiter(opt.QPS),
		sem:      make(chan struct{}, opt.MaxConcurrent),
		headers:  safeHeaders(opt.Headers),
	}
	if c.resolver == nil {
		c.resolver = net.DefaultResolver
	}
	c.hc = &http.Client{
		Timeout:       opt.TotalTimeout,
		Transport:     c.transport(),
		CheckRedirect: c.checkRedirect,
	}
	return c
}

// Get 抓取一个 URL。返回的 Response 在 ErrStatus / ErrTooLarge 时也可能非 nil。
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u, err := c.validateURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := c.validateHost(ctx, u); err != nil {
		return nil, err
	}
	release, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: 构造请求失败: %v", ErrBlocked, err)
	}
	c.applyHeaders(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, classifyError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, truncated, err := readLimited(resp.Body, c.opt.MaxBodyBytes)
	if err != nil {
		return nil, classifyError(ctx, err)
	}
	out := &Response{
		Status:    resp.StatusCode,
		Headers:   resp.Header.Clone(),
		Body:      body,
		FinalURL:  resp.Request.URL.String(),
		Truncated: truncated,
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return out, fmt.Errorf("%w: 状态码 %d", ErrStatus, resp.StatusCode)
	}
	return out, nil
}

// applyHeaders 给请求套上 UA 与书源配置的静态请求头（已清洗，见 headers.go）。
func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.opt.UserAgent)
	for name, values := range c.headers {
		req.Header[name] = append([]string(nil), values...)
	}
}

// readLimited 读取至多 max 字节；多读 1 字节用于判断是否截断。
func readLimited(r io.Reader, max int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > max {
		return body[:max], true, nil
	}
	return body, false, nil
}

// classifyError 把传输层错误归到五类之一。
func classifyError(ctx context.Context, err error) error {
	if errors.Is(err, ErrBlocked) || errors.Is(err, ErrStatus) {
		return err
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return fmt.Errorf("%w: %v", ErrNetwork, err)
}

// withDefaults 补齐零值字段；QPS 的零值取默认 1，负数才表示「关闭限速」。
func withDefaults(o Options) Options {
	d := DefaultOptions()
	o.TotalTimeout = posDuration(o.TotalTimeout, d.TotalTimeout)
	o.ConnectTimeout = posDuration(o.ConnectTimeout, d.ConnectTimeout)
	o.HeaderTimeout = posDuration(o.HeaderTimeout, d.HeaderTimeout)
	o.MaxBodyBytes = posInt64(o.MaxBodyBytes, d.MaxBodyBytes)
	o.MaxHeaderBytes = posInt64(o.MaxHeaderBytes, d.MaxHeaderBytes)
	o.MaxRedirects = posInt(o.MaxRedirects, d.MaxRedirects)
	o.MaxConcurrent = posInt(o.MaxConcurrent, d.MaxConcurrent)
	if o.QPS == 0 {
		o.QPS = d.QPS
	}
	if o.UserAgent == "" {
		o.UserAgent = d.UserAgent
	}
	if len(o.AllowedPorts) == 0 {
		o.AllowedPorts = d.AllowedPorts
	}
	return o
}

func posDuration(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

func posInt64(v, def int64) int64 {
	if v <= 0 {
		return def
	}
	return v
}

func posInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
