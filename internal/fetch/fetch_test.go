package fetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newLocalServer 起一个只监听回环的 httptest 服务。
func newLocalServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// dialerFor 把「公网 IP:端口」路由到本地测试服务，避免任何真实外网请求。
func dialerFor(t *testing.T, routes map[string]string) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		target, ok := routes[port]
		if !ok {
			return nil, fmt.Errorf("测试未配置端口 %s 的路由", port)
		}
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
}

// portOf 取服务端口，并返回供 URL 使用的公网地址。
func portOf(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("取端口失败: %v", err)
	}
	return port, atoi(t, port)
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("端口 %q 不是数字: %v", s, err)
	}
	return n
}

// publicURL 用 IANA 保留的示例地址拼 URL：校验看到的是公网，实际连接走注入的 dialer。
func publicURL(port string) string { return "http://198.51.100.7:" + port + "/" }

func TestGetSuccess(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "ok")
		_, _ = w.Write([]byte("hello"))
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, AllowedPorts: []int{p}, DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	resp, err := c.Get(context.Background(), publicURL(port))
	if err != nil {
		t.Fatalf("期望成功，实际 %v", err)
	}
	if string(resp.Body) != "hello" || resp.Status != 200 || resp.Truncated {
		t.Fatalf("响应不符: %+v", resp)
	}
	if got := resp.Headers.Get("X-Test"); got != "ok" {
		t.Fatalf("响应头缺失: %q", got)
	}
}

func TestBodyTruncated(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, MaxBodyBytes: 1024, AllowedPorts: []int{p},
		DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	resp, err := c.Get(context.Background(), publicURL(port))
	if err != nil {
		t.Fatalf("截断不应报错，实际 %v", err)
	}
	if !resp.Truncated || len(resp.Body) != 1024 {
		t.Fatalf("期望截断到 1024 字节，实际 len=%d truncated=%v", len(resp.Body), resp.Truncated)
	}
}

func TestStatusError(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, AllowedPorts: []int{p}, DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	resp, err := c.Get(context.Background(), publicURL(port))
	if !errors.Is(err, ErrStatus) {
		t.Fatalf("期望 ErrStatus，实际 %v", err)
	}
	if resp == nil || resp.Status != 500 {
		t.Fatalf("非 2xx 时也要返回 Response，实际 %+v", resp)
	}
}

func TestHeaderTimeout(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, HeaderTimeout: 50 * time.Millisecond, AllowedPorts: []int{p},
		DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	_, err := c.Get(context.Background(), publicURL(port))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("期望 ErrTimeout，实际 %v", err)
	}
}

func TestQPSLimit(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: 1, AllowedPorts: []int{p}, DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	start := time.Now()
	if _, err := c.Get(context.Background(), publicURL(port)); err != nil {
		t.Fatalf("第一次请求失败: %v", err)
	}
	if _, err := c.Get(context.Background(), publicURL(port)); err != nil {
		t.Fatalf("第二次请求失败: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 700*time.Millisecond {
		t.Fatalf("QPS=1 时第二次请求应等待约 1s，实际总耗时 %v", elapsed)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var cur, peak int32
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&cur, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		_, _ = w.Write([]byte("ok"))
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, MaxConcurrent: 2, AllowedPorts: []int{p},
		DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()})})

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Get(context.Background(), publicURL(port))
		}()
	}
	wg.Wait()
	if peak > 2 {
		t.Fatalf("并发峰值应 ≤2，实际 %d", peak)
	}
}

func TestRedirectToPrivateBlocked(t *testing.T) {
	var targetPort string
	redirector := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:"+targetPort+"/next", http.StatusFound)
	}))
	target := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("重定向目标不应被访问")
	}))
	port, p := portOf(t, redirector)
	targetPort, tp := portOf(t, target)
	c := New(Options{QPS: -1, AllowedPorts: []int{p, tp},
		DialContext: dialerFor(t, map[string]string{
			port:       redirector.Listener.Addr().String(),
			targetPort: target.Listener.Addr().String(),
		})})

	_, err := c.Get(context.Background(), publicURL(port))
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("重定向到私网应被拒，实际 %v", err)
	}
}

func TestRedirectTooManyHops(t *testing.T) {
	var port string
	loop := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://198.51.100.7:"+port+"/again", http.StatusFound)
	}))
	port, p := portOf(t, loop)
	c := New(Options{QPS: -1, MaxRedirects: 5, AllowedPorts: []int{p},
		DialContext: dialerFor(t, map[string]string{port: loop.Listener.Addr().String()})})

	_, err := c.Get(context.Background(), publicURL(port))
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("超过最大跳数应被拒，实际 %v", err)
	}
}
