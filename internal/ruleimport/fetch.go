package ruleimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"norvels/internal/fetch"
)

// unescape 解码 query 参数（src 可能被百分号编码）。
func unescape(s string) (string, error) {
	return url.QueryUnescape(s)
}

// fetchJSON 取回内容并确认是 JSON；HTML 响应会尝试从 <pre>/<code>/ld+json 里提取。
func fetchJSON(ctx context.Context, source, hint, via string, opts Options) (Result, error) {
	max := opts.MaxBytes
	if max <= 0 {
		max = defaultMaxBytes
	}
	f := opts.Fetcher
	if f == nil {
		f = fetch.New(fetch.Options{MaxBodyBytes: max})
	}
	resp, err := f.Get(ctx, source)
	if err != nil {
		return Result{}, err
	}
	if resp.Truncated || int64(len(resp.Body)) > max {
		return Result{}, fmt.Errorf("%w：响应超过 %d 字节", ErrTooLarge, max)
	}
	body := bytes.TrimSpace(resp.Body)
	if json.Valid(body) {
		return Result{JSON: body, Source: source, Hint: hint, Via: via}, nil
	}
	extracted, ok := extractJSON(body)
	if !ok {
		return Result{}, fmt.Errorf("%w：%s 里没有可用的 JSON", ErrNotJSON, source)
	}
	return Result{JSON: extracted, Source: source, Hint: hint, Via: via}, nil
}

// extractSelectors 是 HTML 里可能内嵌 JSON 的位置，按优先级排列。
var extractSelectors = []string{
	`script[type="application/ld+json"]`,
	"pre",
	"code",
}

// extractJSON 从 HTML 里找第一段合法 JSON。
func extractJSON(body []byte) ([]byte, bool) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, false
	}
	for _, sel := range extractSelectors {
		if found, ok := firstValidJSON(doc, sel); ok {
			return found, true
		}
	}
	return nil, false
}

// firstValidJSON 在指定选择器命中的节点里找第一段合法 JSON。
func firstValidJSON(doc *goquery.Document, sel string) ([]byte, bool) {
	var found []byte
	doc.Find(sel).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		if text != "" && json.Valid([]byte(text)) {
			found = []byte(text)
			return false
		}
		return true
	})
	return found, found != nil
}
