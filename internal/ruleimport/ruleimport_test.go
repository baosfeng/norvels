package ruleimport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norvels/internal/fetch"
)

// newServer 起一个只监听回环的测试服务。
func newServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// publicURL 用 IANA 示例地址拼 URL，实际连接走注入的 dialer。
func publicURL(port string) string { return "http://198.51.100.7:" + port + "/" }

// testFetcher 构造一个把公网地址路由到本地服务的 fetch 客户端。
func testFetcher(t *testing.T, srv *httptest.Server) Fetcher {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return fetch.New(fetch.Options{
		QPS:          -1,
		AllowedPorts: []int{atoi(t, port)},
		MaxBodyBytes: defaultMaxBytes,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	})
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("端口 %q 不是数字", s)
	}
	return n
}

func mustFetch(t *testing.T, input string, opts Options) (Result, error) {
	t.Helper()
	return Fetch(context.Background(), input, opts)
}

func TestInlineJSON(t *testing.T) {
	in := `[{"name":"示例"}]`
	res, err := mustFetch(t, in, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Via != "text" || string(res.JSON) != in {
		t.Fatalf("内联 JSON 结果不符: %+v", res)
	}
	if _, err := mustFetch(t, `{"a":}`, Options{}); !errors.Is(err, ErrNotJSON) {
		t.Fatalf("坏 JSON 应报 ErrNotJSON，实际 %v", err)
	}
}

func TestURLFetch(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	res, err := mustFetch(t, publicURL(port), Options{Fetcher: testFetcher(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Via != "url" || !strings.Contains(string(res.JSON), "ok") {
		t.Fatalf("URL 取回结果不符: %+v", res)
	}
}

func TestLegadoProtocol(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"bookSourceUrl":"https://example.invalid"}]`))
	}))
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	link := "legado://import/bookSource?src=" + publicURL(port)
	res, err := mustFetch(t, link, Options{Fetcher: testFetcher(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Via != "legado" || res.Hint != "bookSource" {
		t.Fatalf("legado 协议解析错误: %+v", res)
	}
}

func TestYueduProtocol(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"a":1}`))
	}))
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	link := "yuedu://booksource/importonline?src=" + publicURL(port)
	res, err := mustFetch(t, link, Options{Fetcher: testFetcher(t, srv)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Via != "yuedu" || res.Hint != "bookSource" {
		t.Fatalf("yuedu 协议解析错误: %+v", res)
	}
}

func TestUnsupportedAndSrcCheck(t *testing.T) {
	bad := []string{
		"hello",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"legado://import/bookSource",
		"legado://import/bookSource?src=ftp://example.invalid/x.json",
		"legado://import/",
	}
	for _, in := range bad {
		if _, err := mustFetch(t, in, Options{}); !errors.Is(err, ErrUnsupportedInput) {
			t.Errorf("%q 应报 ErrUnsupportedInput，实际 %v", in, err)
		}
	}
}

func TestHTMLExtract(t *testing.T) {
	pages := map[string]string{
		"pre":     `<html><body><pre>[{"a":1}]</pre></body></html>`,
		"code":    `<html><body><code>{"b":2}</code></body></html>`,
		"ld+json": `<html><head><script type="application/ld+json">{"c":3}</script></head></html>`,
	}
	for name, html := range pages {
		srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(html))
		}))
		_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
		res, err := mustFetch(t, publicURL(port), Options{Fetcher: testFetcher(t, srv)})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(res.JSON) == 0 {
			t.Fatalf("%s 未提取到 JSON", name)
		}
	}
}

func TestNoJSONInHTML(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>没有 JSON</p></body></html>`))
	}))
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if _, err := mustFetch(t, publicURL(port), Options{Fetcher: testFetcher(t, srv)}); !errors.Is(err, ErrNotJSON) {
		t.Fatalf("应报 ErrNotJSON，实际 %v", err)
	}
}

func TestTooLarge(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"pad":"` + strings.Repeat("x", 2<<20) + `"}`))
	}))
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if _, err := mustFetch(t, publicURL(port), Options{Fetcher: testFetcher(t, srv)}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("应报 ErrTooLarge，实际 %v", err)
	}
}

func TestPrivateURLBlocked(t *testing.T) {
	srv := newServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("私网地址不应被请求")
	}))
	// 默认 Fetcher：真实 fetch 客户端，会做 SSRF 校验。
	if _, err := mustFetch(t, srv.URL, Options{}); !errors.Is(err, fetch.ErrBlocked) {
		t.Fatalf("私网地址应被 fetch 拦下（ErrBlocked），实际 %v", err)
	}
}
