package parser

import (
	"strings"
	"testing"
)

// newDoc 构造一个最小 Doc，供过滤器单测使用。
func newDoc(text string) *Doc {
	return &Doc{Name: "某书.txt", Text: text, opts: DefaultOptions()}
}

// TestFilterBOM 覆盖 bom 过滤器的正常与无 BOM 路径。
func TestFilterBOM(t *testing.T) {
	f := bomFilter{}
	if f.Name() != "bom" || f.Priority() != 10 {
		t.Errorf("name/priority = %q/%d", f.Name(), f.Priority())
	}
	d := newDoc("\ufeff书名")
	if err := f.Apply(d); err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if d.Text != "书名" {
		t.Errorf("Text = %q，期望「书名」", d.Text)
	}
	d = newDoc("没有BOM")
	if err := f.Apply(d); err != nil || d.Text != "没有BOM" {
		t.Errorf("无 BOM 时不应改变: %q %v", d.Text, err)
	}
}

// TestFilterNewline 覆盖换行归一与零宽字符清理。
func TestFilterNewline(t *testing.T) {
	f := newlineFilter{}
	if f.Priority() != 20 {
		t.Errorf("Priority = %d", f.Priority())
	}
	d := newDoc("第一行\r\n第二行\r第三行\u200b尾")
	if err := f.Apply(d); err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if strings.ContainsAny(d.Text, "\r") {
		t.Errorf("仍含回车: %q", d.Text)
	}
	if strings.Contains(d.Text, "\u200b") {
		t.Errorf("仍含零宽空格: %q", d.Text)
	}
	if d.Text != "第一行\n第二行\n第三行尾" {
		t.Errorf("Text = %q", d.Text)
	}
	// 已是 LF 的文本不应被改动。
	d = newDoc("已经是\nLF")
	before := d.Text
	_ = f.Apply(d)
	if d.Text != before {
		t.Errorf("LF 文本被改动: %q", d.Text)
	}
}

// TestFilterNoise 覆盖噪声清洗器的构建与诊断记录。
func TestFilterNoise(t *testing.T) {
	f := noiseFilter{}
	if f.Priority() != 30 {
		t.Fatalf("Priority = %d", f.Priority())
	}
	d := newDoc("正文\n更多小说资源尽在强鹿小说群：123456\n结尾")
	if err := f.Apply(d); err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if d.cl == nil {
		t.Fatal("应构建 cleaner")
	}
	cleaned := d.cl.cleanContentAt(d.Text, 0)
	if strings.Contains(cleaned, "强鹿") || strings.Contains(cleaned, "123456") {
		t.Errorf("广告行未被清除: %q", cleaned)
	}
	if !strings.Contains(cleaned, "正文") {
		t.Errorf("正文被误删: %q", cleaned)
	}
}

// TestFilterWidth 覆盖全角数字归一的正常与「无需处理」路径。
func TestFilterWidth(t *testing.T) {
	f := widthFilter{}
	if f.Priority() != 40 {
		t.Error("Priority 应为 40")
	}
	d := newDoc("第１２３章")
	if err := f.Apply(d); err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if d.Text != "第123章" {
		t.Errorf("Text = %q，期望「第123章」", d.Text)
	}
	// 没有全角数字时应原样返回（走快路径）。
	d = newDoc("第一二三章")
	if err := f.Apply(d); err != nil || d.Text != "第一二三章" {
		t.Errorf("无全角数字时不应改变: %q %v", d.Text, err)
	}
	// 全角字母/标点不应被改（避免改变正文排版）。
	d = newDoc("ＡＢ。")
	_ = f.Apply(d)
	if d.Text != "ＡＢ。" {
		t.Errorf("非数字全角字符被改动: %q", d.Text)
	}
}

// TestRunFiltersOrderAndDisable 覆盖过滤器链的执行顺序与按名关闭。
func TestRunFiltersOrderAndDisable(t *testing.T) {
	d := newDoc("\ufeff第１２３章\r\n正文")
	runFilters(d, nil, nil)
	if d.Text != "第123章\n正文" {
		t.Errorf("过滤器链结果 = %q", d.Text)
	}
	if got := d.Stats["filters"]; got == nil {
		t.Error("过滤链名单未写入诊断")
	}
	// 关闭 width 后全角数字保留。
	d = newDoc("第１２３章")
	runFilters(d, disableSet([]string{"width"}), nil)
	if d.Text != "第１２３章" {
		t.Errorf("关闭 width 后仍被归一: %q", d.Text)
	}
}

// failingFilter 是「永远失败」的过滤器，用于验证降级路径。
type failingFilter struct{}

func (failingFilter) Name() string  { return "boom" }
func (failingFilter) Priority() int { return 5 }
func (failingFilter) Apply(d *Doc) error {
	panic("故意 panic，验证降级")
}

// TestRunFiltersDegradeOnPanic 验证单个过滤器 panic 不影响其余过滤器与解析。
func TestRunFiltersDegradeOnPanic(t *testing.T) {
	d := newDoc("\ufeff书名")
	runFilters(d, nil, []Filter{failingFilter{}})
	if d.Text != "书名" {
		t.Errorf("panic 后其余过滤器应继续生效: %q", d.Text)
	}
	if len(d.Warnings) == 0 {
		t.Error("应记录降级告警")
	}
}
