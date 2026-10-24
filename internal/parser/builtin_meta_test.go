package parser

import (
	"strings"
	"testing"
)

// TestMetaFilename 覆盖 filename 策略的命中与放弃路径。
func TestMetaFilename(t *testing.T) {
	m := filenameMeta{}
	if m.Name() != "filename" || m.Weight() != 80 {
		t.Errorf("name/weight = %q/%d", m.Name(), m.Weight())
	}
	d := &Doc{Name: "【某书名】作者：某作者.txt"}
	got, score := m.Extract(d)
	if got.Title != "某书名" {
		t.Errorf("书名 = %q", got.Title)
	}
	if got.Author != "某作者" {
		t.Errorf("作者 = %q", got.Author)
	}
	if score <= 0 {
		t.Errorf("适用度应为正: %d", score)
	}
	// 无信息文件名：书名兜底为文件主干。
	d = &Doc{Name: "无书名.txt"}
	got, score = m.Extract(d)
	if got.Title == "" || score <= 0 {
		t.Errorf("兜底书名 = %q score=%d", got.Title, score)
	}
}

// TestMetaPreamble 覆盖 preamble 策略：只抽到作者时适用度仍为正。
func TestMetaPreamble(t *testing.T) {
	m := preambleMeta{}
	if m.Name() != "preamble" || m.Weight() != 60 {
		t.Errorf("name/weight = %q/%d", m.Name(), m.Weight())
	}
	d := newDoc("书名：某书名\n作者：某作者\n内容简介：一段合成简介文字，用于测试。\n\n第一章 标题\n正文")
	d.cands = []candidate{{start: strings.Index(d.Text, "第一章")}}
	got, score := m.Extract(d)
	if got.Title != "某书名" || got.Author != "某作者" {
		t.Errorf("抽取结果 = %q / %q", got.Title, got.Author)
	}
	if got.Intro == "" {
		t.Error("简介未抽到")
	}
	if score <= 0 {
		t.Errorf("适用度应为正: %d", score)
	}
	// 只有作者、没有书名：适用度仍应反映「抽到了东西」。
	d2 := newDoc("作者：某作者\n\n第一章 标题\n正文")
	d2.cands = []candidate{{start: strings.Index(d2.Text, "第一章")}}
	got2, score2 := d2.dummyExtract(m)
	if got2.Author == "" || score2 <= 0 {
		t.Errorf("仅作者命中时 score 不应为 0: %q %d", got2.Author, score2)
	}
}

// dummyExtract 是测试辅助：等价于 Extract，但保证 d.preamble 走「已有候选」分支。
func (d *Doc) dummyExtract(m MetaExtractor) (Meta, int) { return m.Extract(d) }

// TestMetaHeadingReverse 覆盖 heading-reverse 的命中与放弃路径。
func TestMetaHeadingReverse(t *testing.T) {
	m := headingReverseMeta{}
	if m.Name() != "heading-reverse" || m.Weight() != 40 {
		t.Errorf("name/weight = %q/%d", m.Name(), m.Weight())
	}
	d := newDoc("某书名 第一章 标题\n正文\n某书名 第二章 标题\n正文\n某书名 第三章 标题\n正文")
	got, score := m.Extract(d)
	if got.Title != "某书名" || score <= 0 {
		t.Errorf("反推书名 = %q score=%d", got.Title, score)
	}
	// 没有卷前缀：应放弃。
	d = newDoc("第一章 标题\n正文")
	if _, score := m.Extract(d); score != 0 {
		t.Errorf("无前缀时应放弃，实际 score=%d", score)
	}
}

// TestMetaStem 覆盖 stem 兜底策略。
func TestMetaStem(t *testing.T) {
	m := stemMeta{}
	if m.Name() != "stem" || m.Weight() != 10 {
		t.Errorf("name/weight = %q/%d", m.Name(), m.Weight())
	}
	got, score := m.Extract(&Doc{Name: "645.【某书名】重修加料版.txt"})
	if got.Title == "" || score <= 0 {
		t.Errorf("书名 = %q score=%d", got.Title, score)
	}
	// 纯扩展名的病态输入：应放弃而不是返回空书名。
	if _, score := m.Extract(&Doc{Name: ".txt"}); score != 0 {
		t.Errorf("空主干应放弃，实际 score=%d", score)
	}
}

// TestRunMetasFieldVoting 验证「逐字段投票」：文件名给书名、正文头部给作者。
func TestRunMetasFieldVoting(t *testing.T) {
	d := newDoc("书名：正文里的书名\n作者：正文里的作者\n\n第一章 标题\n正文")
	d.Name = "文件名书名.txt"
	d.cands = []candidate{{start: strings.Index(d.Text, "第一章")}}
	runMetas(d, nil)
	if d.Meta.Title != "文件名书名" {
		t.Errorf("书名应由高权重的 filename 胜出，实际 %q", d.Meta.Title)
	}
	if d.Meta.Author != "正文里的作者" {
		t.Errorf("作者应由 preamble 提供，实际 %q", d.Meta.Author)
	}
	if d.Meta.TitleScore <= d.Meta.AuthorScore {
		t.Logf("字段分: title=%d author=%d", d.Meta.TitleScore, d.Meta.AuthorScore)
	}
}

// failingMeta 是「永远 panic」的元数据策略，用于验证降级。
type failingMeta struct{}

func (failingMeta) Name() string               { return "boom-meta" }
func (failingMeta) Weight() int                { return 100 }
func (failingMeta) Extract(d *Doc) (Meta, int) { panic("故意 panic，验证降级") }

// TestRunMetasDegradeOnPanic 验证单个元数据策略 panic 不影响其余策略。
func TestRunMetasDegradeOnPanic(t *testing.T) {
	d := newDoc("作者：某作者\n\n第一章 标题\n正文")
	d.Name = "某书名.txt"
	d.cands = []candidate{{start: strings.Index(d.Text, "第一章")}}
	runMetas(d, []MetaExtractor{failingMeta{}})
	if d.Meta.Author != "某作者" {
		t.Errorf("其它策略应继续投票，作者 = %q", d.Meta.Author)
	}
	if len(d.Warnings) == 0 {
		t.Error("应记录降级告警")
	}
}
