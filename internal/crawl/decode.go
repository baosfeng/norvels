package crawl

// 在线书源页面的解码：书源站点大量是 GBK / GB18030，按 UTF-8 解会中文全乱。
//
// 优先级（与任务口径一致）：URL 选项 charset > profile.decode.forceEncoding >
// <meta charset> > UTF-8。解码失败只降级（保留已解出的部分并给 warning），不报错——
// 一个编码判断失误不该让整批检索中断。

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// reMetaCharset 从页面头部抓 <meta charset=...>（也覆盖 http-equiv 的 content 串）。
var reMetaCharset = regexp.MustCompile(`(?i)<meta[^>]{0,512}?charset\s*=\s*["']?\s*([a-z0-9][a-z0-9._-]*)`)

// decodeBody 解码响应体，返回文本、实际使用的编码与降级原因（空表示顺利）。
func decodeBody(body []byte, explicitCharset, forcedCharset string) (string, string, string) {
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	var warnings []string
	for _, hint := range []struct{ source, name string }{
		{"URL 选项", explicitCharset},
		{"decode.forceEncoding", forcedCharset},
	} {
		if strings.TrimSpace(hint.name) == "" {
			continue
		}
		if text, used, warning, known := decodeCharset(body, hint.name); known {
			return text, used, joinedWarnings(warnings, warning)
		}
		warnings = append(warnings, fmt.Sprintf("%s 指定的编码 %q 不支持", hint.source, hint.name))
	}
	if enc := metaCharset(body); enc != "" {
		if text, used, warning, known := decodeCharset(body, enc); known {
			return text, used, joinedWarnings(warnings, warning)
		}
		warnings = append(warnings, fmt.Sprintf("<meta charset> 的编码 %q 不支持", enc))
	}
	if utf8.Valid(body) {
		return string(body), "utf-8", strings.Join(warnings, "；")
	}
	warnings = append(warnings, "未识别页面编码且不是合法 UTF-8，按 UTF-8 兜底")
	return string(body), "utf-8", strings.Join(warnings, "；")
}

// decodeCharset 按编码名解码；known=false 表示不认识这个编码名。
func decodeCharset(body []byte, name string) (text, used, warning string, known bool) {
	switch normalizeCharset(name) {
	case "utf-8":
		return string(body), "utf-8", "", true
	case "gb18030":
		out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(body)
		switch {
		case err != nil:
			return string(out), "gb18030", "GB18030 解码未完全成功，保留已解出的部分", true
		case bytes.ContainsRune(out, utf8.RuneError):
			// x/text 对非法字节替换为 U+FFFD 而不报错：用替换字符的出现标记降级。
			return string(out), "gb18030", "GB18030 解码出现非法字节（已替换为 U+FFFD）", true
		default:
			return string(out), "gb18030", "", true
		}
	default:
		return "", "", "", false
	}
}

// normalizeCharset 归一化编码名（大小写、引号、GBK 家族别名）；不支持时返回空串。
func normalizeCharset(name string) string {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(name), "\"'")) {
	case "utf-8", "utf8", "unicode-1-1-utf-8":
		return "utf-8"
	case "gbk", "gb2312", "gb18030", "gb_2312", "gb_2312-80", "x-gbk", "cp936", "euc-cn":
		return "gb18030"
	}
	return ""
}

// metaCharset 从页面头部（前 4 KB）探测 <meta charset>。
func metaCharset(body []byte) string {
	sample := body
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	m := reMetaCharset.FindSubmatch(sample)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// joinedWarnings 把已收集的降级原因与本次的降级原因拼成一句。
func joinedWarnings(warnings []string, warning string) string {
	if warning != "" {
		warnings = append(warnings, warning)
	}
	return strings.Join(warnings, "；")
}
