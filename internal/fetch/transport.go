package fetch

import (
	"fmt"
	"net"
	"net/http"
)

// transport 构造受限的 http.Transport：
//   - 永不走环境代理（否则本地校验形同虚设）；
//   - 连接/握手/响应头都有超时；
//   - 关掉自动压缩（避免解压炸弹）与 HTTP/2 升级；
//   - 限制响应头体积。
func (c *Client) transport() *http.Transport {
	dial := c.opt.DialContext
	if dial == nil {
		d := &net.Dialer{Timeout: c.opt.ConnectTimeout, Control: c.control}
		dial = d.DialContext
	}
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            dial,
		TLSHandshakeTimeout:    c.opt.ConnectTimeout,
		ResponseHeaderTimeout:  c.opt.HeaderTimeout,
		MaxResponseHeaderBytes: c.opt.MaxHeaderBytes,
		DisableCompression:     true,
		ForceAttemptHTTP2:      false,
		MaxIdleConns:           4,
		MaxConnsPerHost:        2,
	}
}

// checkRedirect 对每一跳重新校验：跳数、协议降级、URL 与解析后的全部 IP。
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.opt.MaxRedirects {
		return fmt.Errorf("%w: 重定向超过 %d 跳", ErrBlocked, c.opt.MaxRedirects)
	}
	prev := via[len(via)-1].URL
	if prev.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: 禁止 https→%s 降级", ErrBlocked, req.URL.Scheme)
	}
	if _, err := c.validateURL(req.URL.String()); err != nil {
		return err
	}
	return c.validateHost(req.Context(), req.URL)
}
