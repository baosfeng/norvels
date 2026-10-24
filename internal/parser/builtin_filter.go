package parser

import (
	"regexp"
	"strings"
)

// ---------- Filter: bom ----------

// bomFilter 剥掉残留的 UTF-8 / UTF-16 BOM 字符。
type bomFilter struct{}

func (bomFilter) Name() string  { return "bom" }
func (bomFilter) Priority() int { return 10 }

func (bomFilter) Apply(d *Doc) error {
	d.Text = strings.TrimPrefix(d.Text, "\ufeff")
	return nil
}

// ---------- Filter: newline ----------

// newlineFilter 把 CRLF / CR 统一成 LF，并去掉零宽字符。
//
// 用 strings.ReplaceAll 而不是逐字符扫描：只有真正出现过的模式才付扫描成本。
type newlineFilter struct{}

func (newlineFilter) Name() string  { return "newline" }
func (newlineFilter) Priority() int { return 20 }

func (newlineFilter) Apply(d *Doc) error {
	s := d.Text
	if strings.Contains(s, "\r\n") {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	if strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	for _, zw := range zeroWidths {
		if strings.Contains(s, zw) {
			s = strings.ReplaceAll(s, zw, "")
		}
	}
	d.Text = s
	return nil
}

// ---------- Filter: noise ----------

// noiseFilter 构建这篇文档的噪声清洗器（书源广告 / 群号 / 网址 / 分隔线）。
//
// 清洗本身发生在切章时按章进行（cleaner.cleanContentAt），它需要知道每段正文
// 在全文中的偏移才能走快路径，所以这一层只负责扫描并缓存 cleaner。
type noiseFilter struct{}

func (noiseFilter) Name() string  { return "noise" }
func (noiseFilter) Priority() int { return 30 }

func (noiseFilter) Apply(d *Doc) error {
	d.cl = newCleaner(d.Text)
	d.Stat("noise_tokens", len(d.cl.lineToks))
	return nil
}

// ---------- Filter: width ----------

// 全角 ASCII（U+FF01-U+FF5E）到半角的换算偏移。
const fullWidthOffset = 0xFEE0

// fullWidthDigits 只归一全角数字，用于避免误改正文里的全角标点。
var reFullWidthSpace = regexp.MustCompile("[\u3000\u00a0]")

// widthFilter 归一全角数字与全角空格。
//
// 只处理数字原因：章节号识别大量依赖行首数字（「第１２３章」「０００１」），
// 而把全角标点也换掉会改变正文排版与原意，收益远小于风险。
type widthFilter struct{}

func (widthFilter) Name() string  { return "width" }
func (widthFilter) Priority() int { return 40 }

func (widthFilter) Apply(d *Doc) error {
	if !strings.ContainsFunc(d.Text, isFullWidthDigit) {
		return nil
	}
	var b strings.Builder
	b.Grow(len(d.Text))
	for _, r := range d.Text {
		if isFullWidthDigit(r) {
			b.WriteRune(r - fullWidthOffset)
			continue
		}
		b.WriteRune(r)
	}
	d.Text = b.String()
	return nil
}

// isFullWidthDigit 判断是否为全角数字 ０-９。
func isFullWidthDigit(r rune) bool { return r >= 0xFF10 && r <= 0xFF19 }

// registerBuiltinFilters 注册全部内置过滤器。
func registerBuiltinFilters() {
	RegisterFilter(bomFilter{})
	RegisterFilter(newlineFilter{})
	RegisterFilter(noiseFilter{})
	RegisterFilter(widthFilter{})
}

// runFilters 按优先级执行过滤器链。
//
// 降级语义：单个过滤器 error 或 panic 只记 warning 并继续下一个——文本少一次
// 变换仍然可以解析，整篇失败则是不可接受的。
func runFilters(d *Doc, disable map[string]bool, extra []Filter) {
	list := append(Filters(), extra...)
	applied := make([]string, 0, len(list))
	for _, f := range list {
		if disable[f.Name()] {
			continue
		}
		name := f.Name()
		_ = guard(d, "filter", name, func() error { return f.Apply(d) })
		applied = append(applied, name)
	}
	d.Stat("filters", applied)
}
