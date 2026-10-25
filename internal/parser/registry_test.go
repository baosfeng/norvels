package parser

import (
	"testing"
)

// stubCodec 是测试用的最小编码策略。
type stubCodec struct{ name string }

func (s stubCodec) Name() string                     { return s.name }
func (s stubCodec) Try(raw []byte) (string, float64) { return string(raw), 1 }

// stubFilter 是测试用的最小过滤器。
type stubFilter struct {
	name     string
	priority int
}

func (s stubFilter) Name() string  { return s.name }
func (s stubFilter) Priority() int { return s.priority }
func (s stubFilter) Apply(d *Doc) error {
	d.Text = "[" + s.name + "]" + d.Text
	return nil
}

// stubMeta 是测试用的最小元数据策略。
type stubMeta struct {
	name   string
	weight int
}

func (s stubMeta) Name() string { return s.name }
func (s stubMeta) Weight() int  { return s.weight }
func (s stubMeta) Extract(d *Doc) (Meta, int) {
	return Meta{Title: s.name, TitleScore: 50}, 50
}

// snapshotRegistry 保存当前注册表快照。
//
// 注册表是包级全局状态：测试里注册的策略会永久留在里面，进而被后续所有
// 解析用例命中（例如一个会改文本的 filter 会给每本书加上前缀）。
// 凡是会写注册表的测试都必须先快照、再 defer 恢复。
func snapshotRegistry() func() {
	codecs := append([]Codec(nil), regCodecs...)
	filters := append([]Filter(nil), regFilters...)
	metas := append([]MetaExtractor(nil), regMetas...)
	splitters := append([]Splitter(nil), regSplitters...)
	return func() {
		regCodecs, regFilters, regMetas, regSplitters = codecs, filters, metas, splitters
	}
}

// TestRegistryBuiltins 验证内置策略清单与文档一致。
func TestRegistryBuiltins(t *testing.T) {
	wantCodecs := []string{EncUTF8, EncUTF8BOM, EncGB18030, EncUTF16LE, EncUTF16BE, EncBig5}
	have := make(map[string]bool, len(Codecs()))
	for _, c := range Codecs() {
		have[c.Name()] = true
	}
	for _, w := range wantCodecs {
		if !have[w] {
			t.Errorf("缺少内置编码策略 %q", w)
		}
	}
	wantFilters := []string{"bom", "newline", "noise", "width"}
	haveF := make(map[string]bool, len(Filters()))
	for _, f := range Filters() {
		haveF[f.Name()] = true
	}
	for _, w := range wantFilters {
		if !haveF[w] {
			t.Errorf("缺少内置过滤器 %q", w)
		}
	}
	if len(Metas()) < 3 {
		t.Errorf("元数据策略数 = %d，期望 ≥ 3", len(Metas()))
	}
	wantSplitters := []string{"di-unit", "special", "numeric", "en-chapter", "unmarked"}
	haveS := make(map[string]bool, len(Splitters()))
	for _, s := range Splitters() {
		haveS[s.Name()] = true
	}
	for _, w := range wantSplitters {
		if !haveS[w] {
			t.Errorf("缺少内置切章策略 %q", w)
		}
	}
}

// TestFiltersSortedByPriority 验证 Filters() 返回的列表已按优先级升序。
func TestFiltersSortedByPriority(t *testing.T) {
	prev := -1 << 30
	for _, f := range Filters() {
		if f.Priority() < prev {
			t.Errorf("过滤器顺序错误：%q 的优先级 %d 小于前一个 %d", f.Name(), f.Priority(), prev)
		}
		prev = f.Priority()
	}
}

// TestMetasSortedByWeight 验证 Metas() 按权重降序返回。
func TestMetasSortedByWeight(t *testing.T) {
	prev := 1 << 30
	for _, m := range Metas() {
		if m.Weight() > prev {
			t.Errorf("元数据策略顺序错误：%q 权重 %d 大于前一个 %d", m.Name(), m.Weight(), prev)
		}
		prev = m.Weight()
	}
}

// TestRegisterUpsert 验证注册表的「同名覆盖」语义与「注册顺序保持」。
func TestRegisterUpsert(t *testing.T) {
	defer snapshotRegistry()()
	base := len(Filters())
	RegisterFilter(stubFilter{name: "zz-test-filter", priority: 999})
	if len(Filters()) != base+1 {
		t.Fatalf("注册后数量 = %d，期望 %d", len(Filters()), base+1)
	}
	// 同名再注册：覆盖而不是追加。
	RegisterFilter(stubFilter{name: "zz-test-filter", priority: 998})
	if len(Filters()) != base+1 {
		t.Errorf("同名注册应覆盖，实际数量 = %d", len(Filters()))
	}
	last := Filters()[len(Filters())-1]
	if last.Name() != "zz-test-filter" || last.Priority() != 998 {
		t.Errorf("覆盖后 = %q/%d", last.Name(), last.Priority())
	}
}

// TestRegisterIgnoresNil 验证注册 nil 不会 panic，也不会污染注册表。
func TestRegisterIgnoresNil(t *testing.T) {
	defer snapshotRegistry()()
	n := len(Codecs())
	RegisterCodec(nil)
	RegisterFilter(nil)
	RegisterMeta(nil)
	RegisterSplitter(nil)
	if len(Codecs()) != n {
		t.Errorf("nil 注册改变了注册表")
	}
}

// TestRegisterCodecAndMeta 覆盖 Codec / Meta 的注册与查询。
func TestRegisterCodecAndMeta(t *testing.T) {
	defer snapshotRegistry()()
	RegisterCodec(stubCodec{name: "zz-test-codec"})
	found := false
	for _, c := range Codecs() {
		if c.Name() == "zz-test-codec" {
			found = true
		}
	}
	if !found {
		t.Error("自定义编码策略未注册成功")
	}
	RegisterMeta(stubMeta{name: "zz-test-meta", weight: 5})
	found = false
	for _, m := range Metas() {
		if m.Name() == "zz-test-meta" {
			found = true
		}
	}
	if !found {
		t.Error("自定义元数据策略未注册成功")
	}
}

// TestRegisterSplitter 覆盖切章策略的注册与查询。
func TestRegisterSplitter(t *testing.T) {
	defer snapshotRegistry()()
	before := len(Splitters())
	RegisterSplitter(stubSplitter{name: "zz-test-split"})
	if len(Splitters()) != before+1 {
		t.Errorf("注册后数量 = %d", len(Splitters()))
	}
	RegisterSplitter(stubSplitter{name: "zz-test-split"})
	if len(Splitters()) != before+1 {
		t.Errorf("同名切章策略应覆盖，实际 = %d", len(Splitters()))
	}
}

// stubSplitter 是测试用的最小切章策略。
type stubSplitter struct{ name string }

func (s stubSplitter) Name() string          { return s.name }
func (s stubSplitter) Detect(d *Doc) float64 { return 0 }
func (s stubSplitter) Split(d *Doc) error    { return nil }

// TestDocHelpers 覆盖 Doc 上的 Warn / Stat 两个诊断写入助手。
func TestDocHelpers(t *testing.T) {
	d := &Doc{}
	d.Warn("问题 %d 号", 1)
	if len(d.Warnings) != 1 || d.Warnings[0] != "问题 1 号" {
		t.Errorf("Warn = %v", d.Warnings)
	}
	d.Stat("k", 1)
	if d.Stats["k"] != 1 {
		t.Errorf("Stat = %v", d.Stats)
	}
	d.Stat("k2", "v")
	if len(d.Stats) != 2 {
		t.Errorf("Stats 应有 2 项，实际 %v", d.Stats)
	}
}

// TestGuardRecover 直接覆盖 guard 的两条失败路径。
func TestGuardRecover(t *testing.T) {
	d := &Doc{}
	err := guard(d, "filter", "x", func() error { panic("boom") })
	if err == nil {
		t.Error("panic 应转成 error")
	}
	if len(d.Warnings) == 0 {
		t.Error("应记 warning")
	}
	err = guard(d, "filter", "y", func() error { return errTest })
	if err == nil {
		t.Error("error 应原样返回")
	}
}

// TestMergeMeta 覆盖逐字段合并的取高与保留逻辑。
func TestMergeMeta(t *testing.T) {
	var m Meta
	mergeMeta(&m, Meta{Title: "甲", TitleScore: 50, Author: "某", AuthorScore: 30}, 1)
	if m.Title != "甲" || m.Author != "某" {
		t.Fatalf("首次合并 = %+v", m)
	}
	// 低分不应覆盖高分。
	mergeMeta(&m, Meta{Title: "乙", TitleScore: 20}, 1)
	if m.Title != "甲" {
		t.Errorf("低分覆盖了高分: %q", m.Title)
	}
	// 零分与空值都不参与合并。
	mergeMeta(&m, Meta{Title: "", TitleScore: 0, Author: "丙", AuthorScore: 0}, 100)
	if m.Author != "某" {
		t.Errorf("零分不应覆盖: %q", m.Author)
	}
	// 权重为 0 时不参与。
	var m2 Meta
	mergeMeta(&m2, Meta{Title: "丁", TitleScore: 90}, 0)
	if m2.Title != "" {
		t.Errorf("权重 0 不应参与投票: %q", m2.Title)
	}
}

// TestScoredScore 覆盖分数归一化的边界。
func TestScoredScore(t *testing.T) {
	if scaledScore(0, 80) != 0 {
		t.Error("零分应归一为 0")
	}
	if scaledScore(50, 0) != 0 {
		t.Error("零权重应归一为 0")
	}
	if got := scaledScore(50, 2); got != 100 {
		t.Errorf("scaledScore = %d，期望 100", got)
	}
}

// errTest 是测试里复用的哨兵错误。
var errTest = errStub("测试错误")

type errStub string

func (e errStub) Error() string { return string(e) }
