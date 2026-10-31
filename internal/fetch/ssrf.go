package fetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"syscall"
)

// blockedRanges 是标准库 Is* 方法没有覆盖、但同样属于「不该被书源访问」的网段。
// 169.254.0.0/16 / 0.0.0.0/8 已被 IsLinkLocalUnicast / IsUnspecified 覆盖，
// 这里显式列出，便于审阅时一眼看全。
var blockedRanges = mustNets(
	"0.0.0.0/8",
	"100.64.0.0/10",  // CGNAT
	"169.254.0.0/16", // 链路本地，含 169.254.169.254 云元数据
	"192.0.0.0/24",   // IETF 协议分配
	"198.18.0.0/15",  // 基准测试
	"fc00::/7",       // IPv6 唯一本地
	"fe80::/10",      // IPv6 链路本地
	"::/128",         // 未指定
)

func mustNets(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		if _, n, err := net.ParseCIDR(s); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// validateURL 做请求前的第一道校验：协议 / userinfo / host / 端口。
func (c *Client) validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: URL 无法解析: %v", ErrBlocked, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: 协议 %q 不允许（只收 http/https）", ErrBlocked, u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: URL 不允许携带 userinfo", ErrBlocked)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: URL 缺少主机名", ErrBlocked)
	}
	if p := u.Port(); p != "" && !c.portAllowed(p) {
		return nil, fmt.Errorf("%w: 端口 %s 不在白名单 %v 内", ErrBlocked, p, c.opt.AllowedPorts)
	}
	return u, nil
}

// portAllowed 判断端口是否在白名单内。
func (c *Client) portAllowed(port string) bool {
	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	for _, allowed := range c.opt.AllowedPorts {
		if allowed == p {
			return true
		}
	}
	return false
}

// validateHost 解析 host 并校验**全部**解析结果：只放行「所有地址都在公网」的主机，
// 任一命中内网即整体拒绝——不给「取第一个地址」留后门。
func (c *Client) validateHost(ctx context.Context, u *url.URL) error {
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return checkIP(ip)
	}
	addrs, err := c.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: 解析 %q 失败: %v", ErrNetwork, host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("%w: %q 没有可用地址", ErrNetwork, host)
	}
	for _, a := range addrs {
		if err := checkIP(a.IP); err != nil {
			return fmt.Errorf("%w（%q 解析到 %s）", err, host, a.IP)
		}
	}
	return nil
}

// checkIP 判断单个地址是否可访问；::ffff:a.b.c.d 先解包成 IPv4 再判。
func checkIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("%w: 空地址", ErrBlocked)
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4 // ::ffff:a.b.c.d 解包成 IPv4 后再判
	} else if v6 := ip.To16(); v6 != nil {
		ip = v6
	}
	if isSpecialIP(ip) {
		return fmt.Errorf("%w: 地址 %s 属于内网/保留段", ErrBlocked, ip)
	}
	for _, n := range blockedRanges {
		if n.Contains(ip) {
			return fmt.Errorf("%w: 地址 %s 命中保留网段 %s", ErrBlocked, ip, n)
		}
	}
	return nil
}

// isSpecialIP 覆盖标准库自带的特殊地址判定。
func isSpecialIP(ip net.IP) bool {
	return ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

// control 是拨号前的第二道校验：拿到真实 socket 地址后再验一次，防 DNS rebinding。
func (c *Client) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: 连接地址 %q 无法拆解", ErrBlocked, address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: 连接地址 %q 不是 IP", ErrBlocked, address)
	}
	if err := checkIP(ip); err != nil {
		return fmt.Errorf("%w（建连时目标为 %s）", err, ip)
	}
	return nil
}
