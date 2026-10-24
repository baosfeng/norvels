package parser

import (
	"strings"
	"testing"
)

// mkDoc 构造一个已完成前置阶段的 Doc（含候选扫描）。
func mkDoc(text string) *Doc {
	d := newDoc(text)
	d.titleCandidates()
	return d
}

// TestSplitterDiUnit 覆盖 di-unit 的命中、候选不足与放弃路径。
func TestSplitterDiUnit(t *testing.T) {
	s := diUnitSplitter{}
	if s.Name() != "di-unit" {
		t.Errorf("Name = %q", s.Name())
	}
	text := "第一章 甲\n正文一\n第二章 乙\n正文二\n第三章 丙\n正文三"
	d := mkDoc(text)
	if score := s.Detect(d); score <= 0 {
		t.Fatalf("应认领，实际 score=%v", score)
	}
	if err := s.Split(d); err != nil {
		t.Fatalf("Split 失败: %v", err)
	}
	if len(d.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3", len(d.Chapters))
	}
	// 只有 1 个强候选：分数应被压低但仍 > 0。
	d = mkDoc("第一章 甲\n正文")
	if score := s.Detect(d); score <= 0 || score > 0.4 {
		t.Errorf("单候选分数 = %v，期望被压低", score)
	}
	// 完全没有章标记：放弃。
	d = mkDoc("只是一段正文，没有任何章节标记。")
	if score := s.Detect(d); score != 0 {
		t.Errorf("无标记时应放弃，实际 %v", score)
	}
	// Split 在没有候选时应返回错误（由调用方降级）。
	if err := s.Split(mkDoc("无标记正文")); err == nil {
		t.Error("无候选时 Split 应返回错误")
	}
}

// TestSplitterSpecial 覆盖 special 的命中与放弃路径。
func TestSplitterSpecial(t *testing.T) {
	s := specialSplitter{}
	if s.Name() != "special" {
		t.Errorf("Name = %q", s.Name())
	}
	d := mkDoc("序章\n正文一\n楔子\n正文二\n尾声\n正文三")
	if score := s.Detect(d); score <= 0 {
		t.Fatalf("应认领，实际 %v", score)
	}
	if err := s.Split(d); err != nil {
		t.Fatalf("Split 失败: %v", err)
	}
	if len(d.Chapters) < 2 {
		t.Errorf("章数 = %d", len(d.Chapters))
	}
	// 只有一个特殊标记：放弃（不足以判定全书用特殊标记分章）。
	d = mkDoc("序章\n正文")
	if score := s.Detect(d); score != 0 {
		t.Errorf("单标记应放弃，实际 %v", score)
	}
}

// TestSplitterNumeric 覆盖 numeric 的命中与放弃路径。
func TestSplitterNumeric(t *testing.T) {
	s := numericSplitter{}
	if s.Name() != "numeric" {
		t.Errorf("Name = %q", s.Name())
	}
	d := mkDoc("1、第一段标题\n正文一\n2、第二段标题\n正文二\n3、第三段标题\n正文三")
	if score := s.Detect(d); score <= 0 {
		t.Fatalf("应认领，实际 %v", score)
	}
	if err := s.Split(d); err != nil {
		t.Fatalf("Split 失败: %v", err)
	}
	if len(d.Chapters) < 2 {
		t.Errorf("章数 = %d", len(d.Chapters))
	}
	d = mkDoc("第一章 甲\n正文一\n第二章 乙\n正文二")
	if score := s.Detect(d); score != 0 {
		t.Errorf("全是强特征时应放弃，实际 %v", score)
	}
}

// TestSplitterEnChapter 覆盖 en-chapter 的命中与放弃路径。
func TestSplitterEnChapter(t *testing.T) {
	s := enChapterSplitter{}
	if s.Name() != "en-chapter" {
		t.Errorf("Name = %q", s.Name())
	}
	d := mkDoc("Chapter 1 Title\nbody one\nChapter 2 Title\nbody two\nChapter 3 Title\nbody three")
	if score := s.Detect(d); score <= 0 {
		t.Fatalf("应认领，实际 %v", score)
	}
	if err := s.Split(d); err != nil {
		t.Fatalf("Split 失败: %v", err)
	}
	if len(d.Chapters) < 2 {
		t.Errorf("章数 = %d", len(d.Chapters))
	}
	d = mkDoc("没有英文标记的中文正文。")
	if score := s.Detect(d); score != 0 {
		t.Errorf("无标记时应放弃，实际 %v", score)
	}
}

// TestSplitterUnmarked 覆盖 unmarked 兜底策略。
func TestSplitterUnmarked(t *testing.T) {
	s := unmarkedSplitter{}
	if s.Name() != "unmarked" {
		t.Errorf("Name = %q", s.Name())
	}
	if score := s.Detect(newDoc("任意")); score <= 0 {
		t.Errorf("兜底策略的 Detect 必须为正，实际 %v", score)
	}
	text := strings.Repeat("这是一段没有章节标记的合成正文，用来验证兜底切分。\n", 400)
	d := newDoc(text)
	d.opts.FallbackChunkChars = 1000
	if err := s.Split(d); err != nil {
		t.Fatalf("Split 失败: %v", err)
	}
	if len(d.Chapters) < 2 {
		t.Fatalf("长文本应被等分，实际章数 = %d", len(d.Chapters))
	}
	if !strings.HasPrefix(d.Chapters[0].Title, "第") {
		t.Errorf("兜底标题 = %q", d.Chapters[0].Title)
	}
}

// TestRunSplitterSelectsBest 覆盖「按 Detect 选最优」。
func TestRunSplitterSelectsBest(t *testing.T) {
	d := mkDoc("第一章 甲\n正文一\n第二章 乙\n正文二\n第三章 丙\n正文三")
	runSplitter(d, "", nil)
	if d.Stats["splitter"] != "di-unit" {
		t.Errorf("选中策略 = %v，期望 di-unit", d.Stats["splitter"])
	}
	if len(d.Chapters) != 3 {
		t.Errorf("章数 = %d", len(d.Chapters))
	}
}

// TestRunSplitterForce 覆盖强制切章与非法策略名两条路径。
func TestRunSplitterForce(t *testing.T) {
	text := strings.Repeat("没有标记的合成正文。\n", 300)
	d := mkDoc(text)
	d.opts.FallbackChunkChars = 1000
	runSplitter(d, "unmarked", nil)
	if len(d.Chapters) < 2 {
		t.Errorf("强制 unmarked 应产出多章，实际 %d", len(d.Chapters))
	}
	// 不存在的策略名：回退到打分流程而不是报错。
	d = mkDoc("第一章 甲\n正文一\n第二章 乙\n正文二\n第三章 丙\n正文三")
	runSplitter(d, "不存在的策略", nil)
	if len(d.Chapters) != 3 {
		t.Errorf("非法策略名应回退到自动选择，实际章数 = %d", len(d.Chapters))
	}
}

// failingSplitter 是「永远 panic」的切章策略，用于验证降级。
type failingSplitter struct{}

func (failingSplitter) Name() string          { return "boom-split" }
func (failingSplitter) Detect(d *Doc) float64 { return 0.9 }
func (failingSplitter) Split(d *Doc) error    { panic("故意 panic，验证降级") }

// TestRunSplitterDegradeOnPanic 验证策略 panic 时回退到兜底切分。
func TestRunSplitterDegradeOnPanic(t *testing.T) {
	d := mkDoc("第一章 甲\n正文一\n第二章 乙\n正文二\n第三章 丙\n正文三")
	d.opts.FallbackChunkChars = 1000
	// 强制走会 panic 的策略：验证「策略炸了也不能让整篇解析失败」。
	runSplitter(d, "boom-split", []Splitter{failingSplitter{}})
	if len(d.Chapters) == 0 {
		t.Fatal("panic 后仍应产出章节")
	}
	if len(d.Warnings) == 0 {
		t.Error("应记录降级告警")
	}
}
