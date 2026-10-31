package parser

import (
	"strings"
	"testing"
)

// TestDeclFilterApply 声明式过滤器对全文做全局替换，未命中时不改文本。
func TestDeclFilterApply(t *testing.T) {
	f, err := NewDeclFilter("广告行", 30, "^广告：.*$", "")
	if err != nil {
		t.Fatalf("NewDeclFilter: %v", err)
	}
	if f.Name() != "广告行" || f.Priority() != 30 {
		t.Errorf("name/priority = %q/%d", f.Name(), f.Priority())
	}
	d := &Doc{Text: "第一行\n广告：示例推广\n第二行\n"}
	if err := f.Apply(d); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if strings.Contains(d.Text, "广告：") {
		t.Errorf("广告行没被清掉: %q", d.Text)
	}
	if !strings.Contains(d.Text, "第一行") || !strings.Contains(d.Text, "第二行") {
		t.Errorf("正文被误改: %q", d.Text)
	}

	d2 := &Doc{Text: "干净正文"}
	if err := f.Apply(d2); err != nil {
		t.Fatalf("Apply 无命中也应成功: %v", err)
	}
	if d2.Text != "干净正文" {
		t.Errorf("无命中时文本被改: %q", d2.Text)
	}
}

// TestDeclFilterBadPattern 坏正则返回 error（由编译层决定跳过整包还是拒绝）。
func TestDeclFilterBadPattern(t *testing.T) {
	if _, err := NewDeclFilter("坏", 10, "^(", ""); err == nil {
		t.Fatal("坏正则应返回 error")
	}
}

// TestDeclFilterUnnamed 未命名规则有兜底名，避免 warning 出现空标识。
func TestDeclFilterUnnamed(t *testing.T) {
	f, err := NewDeclFilter("  ", 10, "x", "")
	if err != nil {
		t.Fatalf("NewDeclFilter: %v", err)
	}
	if f.Name() == "" {
		t.Error("未命名规则应有兜底名")
	}
}
