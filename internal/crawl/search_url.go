package crawl

// 搜索 URL 的拼装：展开模板变量、拆 Legado 选项块、把相对路径拼到 bookSourceUrl 上。
//
// 书源的 searchUrl 允许写相对路径（生产 19 个源里塔读、梧桐中文都是这种写法）；
// 只做变量替换会把 /search?query=... 直接交给 fetch，协议判空后整源恒不可用。

import (
	"net/url"
	"strings"

	"norvels/internal/source"
)

// prepareSearchURL 生成真正要请求的搜索 URL（enrich 与 source test 共用）。
// 返回纯 URL、URL 选项块与降级信息。
func prepareSearchURL(p source.Profile, title string) (string, source.URLOptions, []string) {
	rawURL, opts := source.ParseURLOptions(expandSearchURL(p.Online.SearchURL, title))
	degraded := urlOptionWarnings(p.ID, opts)
	resolved, warning := resolveSearchURL(p.Online.BookSourceURL, rawURL)
	if warning != "" {
		degraded = append(degraded, p.ID+":searchUrl "+warning)
	}
	return resolved, opts, degraded
}

// resolveSearchURL 把相对搜索地址拼到 bookSourceUrl 上。
//
// 绝对 URL 原样放行（不二次改写）；相对路径（含以 / 开头与协议相对 //host/x）
// 用 base 解析成绝对 URL。base 带 #fragment（书源里常见 https://x##类别 的写法）
// 时先剥掉尾巴再解析。base 不可用时保留原样并返回降级说明，不抛错——
// 一个源把地址写错不该让整批检索中断。
func resolveSearchURL(base, raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, ""
	}
	if u, err := url.Parse(raw); err == nil && u.IsAbs() {
		return raw, ""
	}
	b, err := url.Parse(stripURLFragment(base))
	if err != nil || b.Scheme == "" || b.Host == "" {
		return raw, "bookSourceUrl 无法作为 base 解析，搜索地址保持原样"
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw, "搜索地址无法解析，保持原样"
	}
	return b.ResolveReference(ref).String(), ""
}

// stripURLFragment 去掉 URL 的 # 及其后内容（# 在 URL 里只可能是片段）。
func stripURLFragment(raw string) string {
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		return raw[:i]
	}
	return raw
}

// baseURLOf 返回规则求值用的 baseUrl：与搜索地址解析同口径，先剥掉 #fragment。
func baseURLOf(p source.Profile) string {
	if p.Online == nil {
		return ""
	}
	return stripURLFragment(p.Online.BookSourceURL)
}
