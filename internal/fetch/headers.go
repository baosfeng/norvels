package fetch

// 请求头的最后一道兜底清洗。
//
// 主校验在 internal/source（导入/保存时拒绝并把原因回显给用户）；这里再挡一次，
// 因为 Client 也可能被其它调用方直接配置。规则与 source 侧一致：
//   - 键必须是 HTTP token；
//   - 值不能含 CR/LF/NUL 或其它控制字符（防请求头注入）；
//   - Host / Content-Length / Transfer-Encoding / Connection / Proxy-* 等
//     「由传输层决定」的键一律丢弃——Host 只能来自 URL，否则请求可能被指向别处。
//
// 丢弃是静默的：调用方（crawl）负责把源侧校验结果报给用户。

import (
	"net/http"
	"strings"
)

// forbiddenHeaders 是不允许由调用方设置的请求头（键为小写）。
var forbiddenHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true,
	"connection": true, "keep-alive": true, "proxy-connection": true,
	"proxy-authorization": true, "te": true, "trailer": true,
	"upgrade": true, "content-encoding": true,
}

// safeHeaders 把不可信的 header map 转成只含安全项的 http.Header。
func safeHeaders(in map[string]string) http.Header {
	out := make(http.Header, len(in))
	for name, value := range in {
		if !safeHeader(name, value) {
			continue
		}
		out.Set(name, value)
	}
	return out
}

// safeHeader 判断单条请求头能否发送。
func safeHeader(name, value string) bool {
	if name == "" || strings.TrimSpace(value) == "" {
		return false
	}
	if forbiddenHeaders[strings.ToLower(name)] {
		return false
	}
	if !tokenName(name) {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == 0x7f || (c < 0x20 && c != '\t') {
			return false // CR/LF/NUL 等控制字符：防请求头注入
		}
	}
	return true
}

// tokenName 判断键名是否只含 HTTP token 字符。
func tokenName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			continue
		}
		switch c {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '\x60', '|', '~':
			continue
		}
		return false
	}
	return true
}
