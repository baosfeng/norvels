package source

// 书源请求头的安全清洗。
//
// header 的键值来自外部书源配置，按不可信输入处理：
//   - 键必须是合法 HTTP token；
//   - 值里不能出现 CR / LF / NUL 或其它控制字符（防请求头注入）；
//   - Host / Content-Length / Transfer-Encoding / Connection / Proxy-* 等
//     「会被传输层改写」的键一律丢弃——Host 只能由 URL 决定，否则自定义 Host
//     可能把请求指到别处。
//
// 这里只做数据清洗，不发请求；fetch 层还有一道同口径的兜底（最后防线）。

import (
	"sort"
	"strings"
)

// forbiddenHeaderKeys 是「不允许由书源注入」的请求头及原因（键为小写）。
var forbiddenHeaderKeys = map[string]string{
	"host":                "Host 由 URL 决定",
	"content-length":      "请求长度由传输层决定",
	"transfer-encoding":   "传输编码由传输层决定",
	"connection":          "连接管理由传输层决定",
	"keep-alive":          "连接管理由传输层决定",
	"proxy-connection":    "连接管理由传输层决定",
	"proxy-authorization": "代理凭据不允许由书源注入",
	"te":                  "传输编码由传输层决定",
	"trailer":             "传输编码由传输层决定",
	"upgrade":             "协议升级由传输层决定",
	"content-encoding":    "压缩编码由传输层决定",
}

// UnsafeHeaderReason 返回某条请求头不可用的原因；空串表示安全。
// validate 用它把具体原因回显给用户，SanitizeHeaders 用它做丢弃判定。
func UnsafeHeaderReason(name, value string) string {
	if !validHeaderName(name) {
		return "键名不是合法的 HTTP token"
	}
	if reason, ok := forbiddenHeaderKeys[strings.ToLower(name)]; ok {
		return reason
	}
	if strings.TrimSpace(value) == "" {
		return "值为空"
	}
	if !validHeaderValue(value) {
		return "值含 CR/LF 或其它控制字符"
	}
	return ""
}

// SanitizeHeaders 返回只含安全请求头的副本，以及被丢弃的键（排序稳定）。
// 入参 map 不会被修改；空 map 原样返回。
func SanitizeHeaders(in map[string]string) (map[string]string, []string) {
	if len(in) == 0 {
		return in, nil
	}
	clean := make(map[string]string, len(in))
	var rejected []string
	for name, value := range in {
		if UnsafeHeaderReason(name, value) != "" {
			rejected = append(rejected, name)
			continue
		}
		clean[name] = value
	}
	sort.Strings(rejected)
	return clean, rejected
}

// validHeaderName 判断键名是否是 RFC 7230 的 token。
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isTokenByte(name[i]) {
			return false
		}
	}
	return true
}

// isTokenByte 判断单个字节是否属于 HTTP token 字符集。
func isTokenByte(c byte) bool {
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '\x60', '|', '~':
		return true
	}
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// validHeaderValue 判断值里没有 CR/LF、NUL 与其它控制字符（保留横向制表符）。
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == 0x7f || (c < 0x20 && c != '\t') {
			return false
		}
	}
	return true
}
