package main

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"norvels/internal/enrich"
	"norvels/internal/model"
	"norvels/internal/store"
)

// TestParseFields 覆盖 --fields 的解析：合法组合、去空白、未知字段报错、空值报错。
func TestParseFields(t *testing.T) {
	got, err := parseFields("author, intro ,category")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[store.EnrichFieldAuthor] || !got[store.EnrichFieldIntro] || !got[store.EnrichFieldCategory] {
		t.Fatalf("三个字段都应识别，实际 %v", got)
	}
	if _, err := parseFields("author,nope"); err == nil {
		t.Fatal("未知字段应报错")
	}
	if _, err := parseFields(" , "); err == nil {
		t.Fatal("空字段列表应报错")
	}
}

// TestNewBatchID 覆盖批次号形状：时间戳-随机后缀，且两次不同。
func TestNewBatchID(t *testing.T) {
	a, b := newBatchID(), newBatchID()
	if a == b {
		t.Fatalf("两次生成的批次号不该相同：%s", a)
	}
	parts := strings.Split(a, "-")
	if len(parts) != 3 {
		t.Fatalf("批次号应形如 20260101-120000-abcdef，实际 %q", a)
	}
	if len(parts[0]) != 8 || len(parts[1]) != 6 || len(parts[2]) != 6 {
		t.Fatalf("批次号各段长度不符: %q", a)
	}
}

// TestValidateEnrichFlags 覆盖参数校验：冲突组合与负数 --flush-every 报错，正常组合放行。
func TestValidateEnrichFlags(t *testing.T) {
	if err := validateEnrichFlags(true, true, 200); err == nil {
		t.Fatal("--apply 与 --read-only 同时用应报错")
	}
	if err := validateEnrichFlags(false, false, -1); err == nil {
		t.Fatal("--flush-every 为负应报错")
	}
	for _, flush := range []int{0, 1, 200} {
		if err := validateEnrichFlags(true, false, flush); err != nil {
			t.Fatalf("--flush-every %d 应合法，实际 %v", flush, err)
		}
	}
}

// TestSortedSkipKeys 覆盖跳过原因排序：输出必须稳定（报告可对照）。
func TestSortedSkipKeys(t *testing.T) {
	got := sortedSkipKeys(map[string]int{"b": 1, "a": 2, "c": 3})
	if !sort.StringsAreSorted(got) || len(got) != 3 {
		t.Fatalf("应按字典序返回全部键，实际 %v", got)
	}
}

// TestFirstSource 覆盖来源取值：空清单回落 unknown。
func TestFirstSource(t *testing.T) {
	if got := firstSource(nil); got != "unknown" {
		t.Fatalf("空来源应回落 unknown，实际 %q", got)
	}
	if got := firstSource([]string{"src-a", "src-b"}); got != "src-a" {
		t.Fatalf("应取第一个来源，实际 %q", got)
	}
}

// TestHints 覆盖两种写库状态提示：dry-run 必须明确说「未写库」。
func TestHints(t *testing.T) {
	if !strings.Contains(applyHint(false), "dry-run") {
		t.Fatalf("非 apply 应提示 dry-run，实际 %q", applyHint(false))
	}
	if strings.Contains(applyHint(true), "dry-run") {
		t.Fatalf("apply 不该再提示 dry-run，实际 %q", applyHint(true))
	}
	if dryHint(true) != "" {
		t.Fatalf("apply 时不该有 dry-run 提示，实际 %q", dryHint(true))
	}
}

// TestSubjectOf 覆盖候选到决策输入的字段搬运。
func TestSubjectOf(t *testing.T) {
	s := subjectOf(store.EnrichCandidate{Title: "示例", Author: "佚名", Intro: "简介",
		CategoryL1: "未分类", CategoryL2: ""})
	if s.Title != "示例" || s.Author != "佚名" || s.CategoryL1 != "未分类" {
		t.Fatalf("字段搬运不完整: %+v", s)
	}
}

// TestMaxInt 覆盖取大值。
func TestMaxInt(t *testing.T) {
	if maxInt(3, 5) != 5 || maxInt(9, 2) != 9 || maxInt(1, 1) != 1 {
		t.Fatal("maxInt 取较大值失败")
	}
}

// TestDecideChangesNoWrite 覆盖 dry-run 语义：只摊清单，不碰库。
func TestDecideChangesNoWrite(t *testing.T) {
	author := enrich.FieldValue{Value: "作者甲", Confidence: 1, Sources: []string{"src-a"}, Changed: true}
	dec := enrich.Decision{Author: &author}
	changes := decideChanges(store.EnrichCandidate{BookID: 7, Author: "佚名"}, dec)
	if len(changes) != 1 || changes[0].NewValue != "作者甲" {
		t.Fatalf("dry-run 应产出变更清单，实际 %+v", changes)
	}
	if changes[0].Field != store.EnrichFieldAuthor || changes[0].BookID != 7 {
		t.Fatalf("变更字段不对: %+v", changes[0])
	}
}

// TestDecideChangesSkipsUnchanged 覆盖「值没变就不记」。
func TestDecideChangesSkipsUnchanged(t *testing.T) {
	author := enrich.FieldValue{Value: "作者甲", Changed: false}
	dec := enrich.Decision{Author: &author}
	if got := decideChanges(store.EnrichCandidate{BookID: 1}, dec); len(got) != 0 {
		t.Fatalf("未变化不该产生变更，实际 %+v", got)
	}
}

// TestEnrichPlanOnTempDB 覆盖 --plan 的端到端：临时库建表后可查到待补清单。
func TestEnrichPlanOnTempDB(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	seedCandidateBooks(t, st)
	cands, err := collectCandidates(t.Context(), st, map[string]bool{
		store.EnrichFieldAuthor: true, store.EnrichFieldIntro: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("种子书应落在待补清单里")
	}
}

// TestCollectCandidatesDedupes 覆盖跨类去重：一本书只出现一次。
func TestCollectCandidatesDedupes(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	seedCandidateBooks(t, st)
	cands, err := collectCandidates(t.Context(), st, map[string]bool{
		store.EnrichFieldAuthor: true, store.EnrichFieldIntro: true, store.EnrichFieldCategory: true,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, c := range cands {
		if seen[c.BookID] {
			t.Fatalf("书 %d 在候选清单里出现了多次", c.BookID)
		}
		seen[c.BookID] = true
	}
}

// seedCandidateBooks 造一本待补书：占位作者 + 无简介 + 未分类，返回书 id。
func seedCandidateBooks(t *testing.T, st *store.Store) int64 {
	t.Helper()
	book := &model.ParsedBook{
		Title:      "修仙示例录",
		Author:     "佚名",
		SourcePath: "/books/demo-seed.txt",
		SourceName: "demo-seed.txt",
		Encoding:   "utf-8",
		FileSize:   1024,
		SHA256:     strings.Repeat("a", 64),
		WordCount:  30,
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: "示例正文。", WordCount: 5},
		},
	}
	res, err := st.SaveBook(t.Context(), book, store.SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return res.BookID
}

// openTempStore 在临时目录里开一个跑完迁移的库。
func openTempStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	st, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MigrateTo(t.Context(), store.LatestVersion()); err != nil {
		st.Close()
		t.Fatal(err)
	}
	return st
}
