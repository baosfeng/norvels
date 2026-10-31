// Package ruleimport 把用户给出的「任何分享站导入入口」统一取回成一段 JSON。
//
// 支持四种输入形态（按顺序判定）：
//   - 直接粘贴的 JSON 文本（以 { 或 [ 开头）
//   - http(s) 直链
//   - legado://import/<type>?src=<url>（书源 / 净化 / TXT 规则 / 订阅源 / 主题…）
//   - yuedu://booksource/importonline?src=<url>（旧版「阅读」协议）
//
// 本包只负责「取回并确认是 JSON」：不判断类型、不落库；SSRF 校验完全复用 internal/fetch，
// 不重复实现一套。
package ruleimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"norvels/internal/fetch"
)

// 错误分类：调用方用 errors.Is 判断。
var (
	// ErrUnsupportedInput 表示输入既不是 JSON 文本，也不是受支持的链接形态。
	ErrUnsupportedInput = errors.New("ruleimport: 不支持的输入形态")
	// ErrNotJSON 表示取回的内容里找不到 JSON（含 HTML 页提取失败）。
	ErrNotJSON = errors.New("ruleimport: 取回内容不是 JSON")
	// ErrTooLarge 表示取回内容超过上限。
	ErrTooLarge = errors.New("ruleimport: 内容超过上限")
)

// defaultMaxBytes 是取回内容上限（1 MiB）。
const defaultMaxBytes = 1 << 20

// Options 控制取回行为；零值可用。
type Options struct {
	// Fetcher 是取回实现（默认用 internal/fetch 的受限客户端）。
	Fetcher Fetcher
	// MaxBytes 是内容上限，默认 1 MiB。
	MaxBytes int64
}

// Fetcher 是取回一段响应体的最小接口，fetch.Client 满足它。
type Fetcher interface {
	Get(ctx context.Context, rawURL string) (*fetch.Response, error)
}

// Result 是一次取回的结果。
type Result struct {
	JSON   []byte // 确认过的 JSON 文本
	Source string // 实际取回的 URL；内联文本为空
	Hint   string // 协议里声明的类型（bookSource/replaceRule/...），仅透传
	Via    string // text / url / legado / yuedu
}

// Fetch 识别输入形态并取回 JSON。
func Fetch(ctx context.Context, input string, opts Options) (Result, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Result{}, fmt.Errorf("%w：输入为空", ErrUnsupportedInput)
	}
	if looksJSON(raw) {
		if !json.Valid([]byte(raw)) {
			return Result{}, fmt.Errorf("%w：看起来像 JSON 但无法解析", ErrNotJSON)
		}
		return Result{JSON: []byte(raw), Via: "text"}, nil
	}
	source, hint, via, err := resolve(raw)
	if err != nil {
		return Result{}, err
	}
	return fetchJSON(ctx, source, hint, via, opts)
}

// looksJSON 判断是否像一段完整的 JSON 文本。
func looksJSON(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

// resolve 把链接形态化成 (src, hint, via)。
func resolve(raw string) (string, string, string, error) {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return raw, "", "url", nil
	case strings.HasPrefix(lower, "legado://import/"):
		hint, src, err := parseLegado(raw)
		if err != nil {
			return "", "", "", err
		}
		return src, hint, "legado", nil
	case strings.HasPrefix(lower, "yuedu://booksource/importonline"):
		query := raw
		if i := strings.Index(raw, "?"); i >= 0 {
			query = raw[i:]
		}
		src, err := parseSrcParam(query)
		if err != nil {
			return "", "", "", err
		}
		return src, "bookSource", "yuedu", nil
	default:
		return "", "", "", fmt.Errorf("%w：支持 JSON 文本、http(s) 链接、legado://import/<type>?src=、yuedu://booksource/importonline?src=", ErrUnsupportedInput)
	}
}

// parseLegado 解析 legado://import/<type>?src=<url>。
func parseLegado(raw string) (hint, src string, err error) {
	rest := strings.TrimPrefix(raw, "legado://import/")
	if rest == raw {
		return "", "", fmt.Errorf("%w：不是 legado 导入链接", ErrUnsupportedInput)
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		hint = strings.Trim(rest[:i], "/")
		rest = rest[i:]
	} else {
		hint = strings.Trim(rest, "/")
		rest = ""
	}
	if hint == "" {
		return "", "", fmt.Errorf("%w：链接缺少类型", ErrUnsupportedInput)
	}
	src, err = parseSrcParam(rest)
	return hint, src, err
}

// parseSrcParam 从查询串里取 src，并校验它是 http(s)。
func parseSrcParam(query string) (string, error) {
	q := strings.TrimPrefix(query, "?")
	for _, pair := range strings.Split(q, "&") {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k != "src" {
			continue
		}
		url, err := unescape(v)
		if err != nil {
			return "", fmt.Errorf("%w：src 参数编码无效", ErrUnsupportedInput)
		}
		if !isHTTP(url) {
			return "", fmt.Errorf("%w：src 只允许 http(s)，实际 %q", ErrUnsupportedInput, url)
		}
		return url, nil
	}
	return "", fmt.Errorf("%w：链接缺少 src 参数", ErrUnsupportedInput)
}

func isHTTP(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}
