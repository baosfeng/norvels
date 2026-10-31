package parser

import "testing"

// TestDeclMetaExtract 声明式元数据规则从正文头部按捕获组取字段，置信分等于规则权重。
func TestDeclMetaExtract(t *testing.T) {
	m, err := NewDeclMeta("头部作者", 60, "^作者：(.+)$", 0, 1, 0)
	if err != nil {
		t.Fatalf("NewDeclMeta: %v", err)
	}
	if m.Name() != "头部作者" || m.Weight() != 60 {
		t.Errorf("name/weight = %q/%d", m.Name(), m.Weight())
	}
	d := &Doc{Text: "作者：示例作者\n书名：示例书名\n第1章 开始\n正文", pre: "作者：示例作者\n书名：示例书名\n"}
	got, score := m.Extract(d)
	if got.Author != "示例作者" || got.AuthorScore == 0 {
		t.Errorf("author = %q/%d，期望示例作者", got.Author, got.AuthorScore)
	}
	if score != 60 {
		t.Errorf("置信分 = %d，期望 60", score)
	}
	if got.Title != "" || got.Intro != "" {
		t.Errorf("未指定的字段应留空: %+v", got)
	}
}

// TestDeclMetaNoMatch 头部不命中时放弃（返回 0 分，不参与投票）。
func TestDeclMetaNoMatch(t *testing.T) {
	m, err := NewDeclMeta("头部作者", 60, "^作者：(.+)$", 0, 1, 0)
	if err != nil {
		t.Fatalf("NewDeclMeta: %v", err)
	}
	d := &Doc{Text: "没有头部信息", pre: "没有头部信息"}
	if _, score := m.Extract(d); score != 0 {
		t.Errorf("无命中置信分 = %d，期望 0", score)
	}
}

// TestDeclMetaRejects 坏正则与越界捕获组都在编译期拒绝。
func TestDeclMetaRejects(t *testing.T) {
	if _, err := NewDeclMeta("坏", 60, "^(", 0, 1, 0); err == nil {
		t.Fatal("坏正则应返回 error")
	}
	if _, err := NewDeclMeta("越界", 60, "^作者：(.+)$", 0, 2, 0); err == nil {
		t.Fatal("捕获组越界应返回 error")
	}
	// 0 = 不使用该字段，不算越界。
	if _, err := NewDeclMeta("零", 60, "^作者：(.+)$", 0, 0, 0); err != nil {
		t.Fatalf("全 0 捕获组应合法: %v", err)
	}
}
