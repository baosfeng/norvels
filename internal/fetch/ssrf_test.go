package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
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

// validateHostOf 让「解析结果 → 主机级判定」这条路径可单独断言，不建连、不出网。
func validateHostOf(t *testing.T, host string, ips []net.IP) error {
	t.Helper()
	c := New(Options{QPS: -1, Resolver: fakeResolver{addrs: map[string][]net.IP{host: ips}}})
	return c.validateHost(context.Background(), mustURL(t, "http://"+host+"/"))
}

// mustIPs 把字符串列表转成 net.IP 列表，顺带校验测试数据本身合法。
func mustIPs(t *testing.T, raws ...string) []net.IP {
	t.Helper()
	out := make([]net.IP, 0, len(raws))
	for _, raw := range raws {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("测试数据 %q 不是合法 IP", raw)
		}
		out = append(out, ip)
	}
	return out
}

// TestUnspecifiedAddressSkipped 是本次修复的主场景：DNS 返回「未指定地址 + 公网地址」
// 时，未指定地址只是噪声，跳过它之后其余地址全在公网 → 放行。
func TestUnspecifiedAddressSkipped(t *testing.T) {
	cases := []struct {
		name string
		ips  []string
	}{
		{"IPv6 未指定 + 公网 IPv4", []string{"::", "198.51.100.7"}},
		{"IPv6 未指定 + 非黑名单 IPv6", []string{"::", "2001:db8::7"}}, // 2001:db8::/32 为文档段，合成用例
		{"IPv4 未指定 + 公网 IPv4", []string{"0.0.0.0", "198.51.100.7"}},
		{"IPv4 映射未指定 + 公网 IPv4", []string{"::ffff:0.0.0.0", "198.51.100.7"}},
		{"未指定在前或在后都不影响", []string{"198.51.100.7", "::"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateHostOf(t, "public.invalid", mustIPs(t, tc.ips...)); err != nil {
				t.Fatalf("未指定地址应被跳过，期望放行，实际 %v", err)
			}
		})
	}
}

// TestUnspecifiedSkippedButPrivateStillBlocked 锁定「没有放松」：未指定地址跳过后，
// 其余地址里的真内网仍是「任一命中即整机拒绝」，顺序不影响结论。
func TestUnspecifiedSkippedButPrivateStillBlocked(t *testing.T) {
	cases := []struct {
		name string
		ips  []string
	}{
		{"未指定 + 回环", []string{"::", "127.0.0.1"}},
		{"回环 + 未指定（顺序无关）", []string{"127.0.0.1", "::"}},
		{"未指定 + 私网 10/8", []string{"::", "10.0.0.9"}},
		{"未指定 + 云元数据", []string{"0.0.0.0", "169.254.169.254"}},
		{"未指定 + IPv6 唯一本地", []string{"::", "fc00::9"}},
		{"公网 + 内网多地址（DNS rebinding 核心语义）", []string{"198.51.100.7", "10.0.0.9"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHostOf(t, "mixed.invalid", mustIPs(t, tc.ips...))
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("期望 ErrBlocked，实际 %v", err)
			}
			if !strings.Contains(err.Error(), "内网/保留段") {
				t.Fatalf("应报命中内网，实际 %v", err)
			}
		})
	}
}

// TestOnlyUnspecifiedRejected 跳过后一个可用地址都不剩：仍然拒绝，但文案是
// 「没有可用地址」而不是「内网」——探活要能分辨这两种失败。
func TestOnlyUnspecifiedRejected(t *testing.T) {
	for _, raw := range []string{"::", "0.0.0.0", "::ffff:0.0.0.0"} {
		t.Run(raw, func(t *testing.T) {
			err := validateHostOf(t, "noise.invalid", mustIPs(t, raw))
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("期望 ErrBlocked，实际 %v", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "没有可用地址") {
				t.Fatalf("应报「没有可用地址」，实际 %v", err)
			}
			if strings.Contains(msg, "内网/保留段") {
				t.Fatalf("未指定地址不应被描述为内网，实际 %v", err)
			}
		})
	}
}

// TestControlStillBlocksUnspecified 锁定放宽的边界：跳过只发生在主机名解析层，
// 真正建连的 socket 地址仍由 control 拒绝未指定地址（Linux 上 connect 0.0.0.0
// 等价于连本机，放过去就是 SSRF）。
func TestControlStillBlocksUnspecified(t *testing.T) {
	c := New(Options{QPS: -1})
	for _, addr := range []string{"0.0.0.0:80", "[::]:80", "[::ffff:0.0.0.0]:80"} {
		if err := c.control("tcp", addr, nil); !errors.Is(err, ErrBlocked) {
			t.Fatalf("control 应拒绝未指定地址 %s，实际 %v", addr, err)
		}
	}
}

// TestGetSucceedsWithUnspecifiedNoise 是线上探活场景的端到端复现：
// 域名同时解析出 :: 与公网 IPv4 时，Get 必须真正发得出去（修复前第一步就判死）。
// 连接走注入的 dialer 指向本地测试服务，不出网。
func TestGetSucceedsWithUnspecifiedNoise(t *testing.T) {
	srv := newLocalServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	port, p := portOf(t, srv)
	c := New(Options{QPS: -1, AllowedPorts: []int{p},
		Resolver:    fakeResolver{addrs: map[string][]net.IP{"mixed.invalid": mustIPs(t, "::", "198.51.100.7")}},
		DialContext: dialerFor(t, map[string]string{port: srv.Listener.Addr().String()}),
	})
	resp, err := c.Get(context.Background(), "http://mixed.invalid:"+port+"/")
	if err != nil {
		t.Fatalf(":: 只是解析噪声，不应阻断请求，实际 %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Fatalf("响应体 = %q", resp.Body)
	}
}

// TestOnlyUnspecifiedRejectedFromLiteralHost 同一条判定对 URL 里的 IP 字面量也成立
// （http://[::]/ 不经 DNS，走的是 validateHost 的字面量分支）。
func TestOnlyUnspecifiedRejectedFromLiteralHost(t *testing.T) {
	c := New(Options{QPS: -1})
	_, err := c.Get(context.Background(), "http://[::]/")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("期望 ErrBlocked，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "没有可用地址") {
		t.Fatalf("应报「没有可用地址」，实际 %v", err)
	}
}
