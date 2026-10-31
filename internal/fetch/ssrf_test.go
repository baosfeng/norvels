package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"
)

// fakeResolver 把域名映射到固定地址，避免测试触网。
type fakeResolver struct {
	addrs map[string][]net.IP
}

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := f.addrs[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	out := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("解析 %q 失败: %v", raw, err)
	}
	return u
}

// TestBlockedAddressMatrix 是 SSRF 黑名单矩阵：11 种内网/保留地址形态
// 全部必须被解析层拒绝，且错误分类为 ErrBlocked。
func TestBlockedAddressMatrix(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"回环-IPv4", "127.0.0.1"},
		{"回环-IPv4-其它", "127.1.2.3"},
		{"私网-10/8", "10.0.0.1"},
		{"私网-172.16/12", "172.16.5.4"},
		{"私网-192.168/16", "192.168.1.1"},
		{"云元数据-169.254.169.254", "169.254.169.254"},
		{"链路本地-169.254.1.1", "169.254.1.1"},
		{"回环-IPv6", "::1"},
		{"IPv6-唯一本地-fc00::/7", "fc00::1"},
		{"IPv6-链路本地-fe80::/10", "fe80::1"},
		{"IPv4-映射-::ffff:127.0.0.1", "::ffff:127.0.0.1"},
		{"未指定-0.0.0.0", "0.0.0.0"},
		{"CGNAT-100.64/10", "100.64.0.1"},
		{"IETF-192.0.0.0/24", "192.0.0.1"},
		{"基准测试-198.18/15", "198.18.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("测试数据 %q 不是合法 IP", tc.ip)
			}
			c := New(Options{QPS: -1, Resolver: fakeResolver{addrs: map[string][]net.IP{
				"evil.invalid": {ip},
			}}})
			_, err := c.Get(context.Background(), "http://evil.invalid/")
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("期望 ErrBlocked，实际 %v", err)
			}
		})
	}
}

// TestBlockedLiteralHost 覆盖「URL 里直接写 IP 字面量」这条路径（不经 DNS）。
func TestBlockedLiteralHost(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/", "http://10.1.2.3/", "http://[::1]/", "http://[fe80::1]/"} {
		t.Run(raw, func(t *testing.T) {
			c := New(Options{QPS: -1})
			if _, err := c.Get(context.Background(), raw); !errors.Is(err, ErrBlocked) {
				t.Fatalf("%q 应被拒，实际 %v", raw, err)
			}
		})
	}
}

// TestProtocolRejected 覆盖协议白名单。
func TestProtocolRejected(t *testing.T) {
	urls := []string{
		"file:///etc/passwd",
		"gopher://127.0.0.1:6379/_INFO",
		"data:text/plain,hello",
		"ftp://example.invalid/file",
		"javascript:alert(1)",
	}
	for _, raw := range urls {
		t.Run(raw, func(t *testing.T) {
			c := New(Options{QPS: -1})
			if _, err := c.Get(context.Background(), raw); !errors.Is(err, ErrBlocked) {
				t.Fatalf("%q 应被拒，实际 %v", raw, err)
			}
		})
	}
}

// TestUserinfoAndPortRejected 覆盖 userinfo 与端口白名单。
func TestUserinfoAndPortRejected(t *testing.T) {
	c := New(Options{QPS: -1})
	if _, err := c.Get(context.Background(), "http://user:pass@example.invalid/"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("userinfo 应被拒，实际 %v", err)
	}
	if _, err := c.Get(context.Background(), "http://example.invalid:8080/"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("8080 端口应被拒，实际 %v", err)
	}
}

// TestResolverFailureIsNetwork 解析失败归类为 ErrNetwork，不是 ErrBlocked。
func TestResolverFailureIsNetwork(t *testing.T) {
	c := New(Options{QPS: -1, Resolver: fakeResolver{}})
	_, err := c.Get(context.Background(), "http://no-such-host.invalid/")
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("期望 ErrNetwork，实际 %v", err)
	}
}

// TestControlBlocksDial 覆盖第二道：建连前的 sockaddr 校验（防 DNS rebinding）。
func TestControlBlocksDial(t *testing.T) {
	c := New(Options{QPS: -1})
	if err := c.control("tcp", "127.0.0.1:8080", nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("control 应拒绝 127.0.0.1，实际 %v", err)
	}
	if err := c.control("tcp", "169.254.169.254:80", nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("control 应拒绝云元数据地址，实际 %v", err)
	}
	if err := c.control("tcp", "198.51.100.7:443", nil); err != nil {
		t.Fatalf("公网地址不应被拒，实际 %v", err)
	}
	if err := c.control("tcp", "not-an-ip:80", nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("非 IP 地址应被拒，实际 %v", err)
	}
}

// TestControlBlocksRealDial 用真实 net.Dialer 验证 Control 生效：
// 目标是本地 httptest，连接必须在建连阶段就被拒绝（不发请求、不出网）。
func TestControlBlocksRealDial(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("请求不应到达服务端")
	}))
	defer srv.Close()

	c := New(Options{QPS: -1})
	d := &net.Dialer{Control: c.control}
	conn, err := d.DialContext(context.Background(), "tcp", srv.Listener.Addr().String())
	if err == nil {
		_ = conn.Close()
		t.Fatal("期望建连被 Control 拒绝，实际成功")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("期望 ErrBlocked，实际 %v", err)
	}
}

// TestRedirectChecks 覆盖重定向的三条规则（单元级，不建连）。
func TestRedirectChecks(t *testing.T) {
	c := New(Options{QPS: -1})
	via := func(n int) []*http.Request {
		out := make([]*http.Request, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, &http.Request{URL: mustURL(t, "https://example.invalid/page")})
		}
		return out
	}

	t.Run("https→http 降级", func(t *testing.T) {
		req := &http.Request{URL: mustURL(t, "http://example.invalid/next")}
		if err := c.checkRedirect(req, via(1)); !errors.Is(err, ErrBlocked) {
			t.Fatalf("期望 ErrBlocked，实际 %v", err)
		}
	})
	t.Run("超过最大跳数", func(t *testing.T) {
		req := &http.Request{URL: mustURL(t, "https://example.invalid/next")}
		if err := c.checkRedirect(req, via(6)); !errors.Is(err, ErrBlocked) {
			t.Fatalf("期望 ErrBlocked，实际 %v", err)
		}
	})
	t.Run("重定向到私网", func(t *testing.T) {
		req := &http.Request{URL: mustURL(t, "http://127.0.0.1/next")}
		if err := c.checkRedirect(req, via(1)); !errors.Is(err, ErrBlocked) {
			t.Fatalf("期望 ErrBlocked，实际 %v", err)
		}
	})
}
