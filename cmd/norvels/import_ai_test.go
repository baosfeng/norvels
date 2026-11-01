package main

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/ai"
	"norvels/internal/model"
)

// fakeCurator 是 ai.Curator 的替身：按顺序回放预设结论。
type fakeCurator struct {
	want    []ai.Curation
	calls   int
	lastErr error
}

func (f *fakeCurator) Curate(context.Context, ai.BookCard) (ai.Curation, error) {
	if f.lastErr != nil {
		return ai.Curation{}, f.lastErr
	}
	got := f.want[0]
	f.want = f.want[1:]
	f.calls++
	return got, nil
}

func TestVerdictOfMapsThreeStates(t *testing.T) {
	if v := verdictOf(ai.Curation{Decision: ai.DecisionImport}); !v.Import {
		t.Fatalf("import 应放行，得到 %+v", v)
	}
	skip := verdictOf(ai.Curation{Decision: ai.DecisionSkip, Reasons: []string{"残卷"}})
	if skip.Import || !strings.Contains(skip.Reason, "残卷") {
		t.Fatalf("skip 应拦下并带理由，得到 %+v", skip)
	}
	// review 也拦下：宁可少导一本，也不要把拿不准的书悄悄塞进书架。
	review := verdictOf(ai.Curation{Decision: ai.DecisionReview, Reasons: []string{"拿不准"}})
	if review.Import || !strings.Contains(review.Reason, "人工确认") {
		t.Fatalf("review 应转人工，得到 %+v", review)
	}
}

func TestCardCarriesEvidence(t *testing.T) {
	a := &aiCuration{categories: []string{"武侠"}}
	book := &model.ParsedBook{
		Title: "甲书", Authors: []string{"作者一", "作者二"}, SourcePath: "/books/甲书.txt",
		Encoding: "gb18030", FileSize: 1024, WordCount: 900, SourceFormat: "",
		Quality:  model.QualityMetrics{NoiseRatio: 0.1, GarbageRatio: 0.02, StructureScore: 0.8},
		Chapters: []model.ParsedChapter{{Content: ""}, {Content: "第一段"}, {Content: "第二段"}},
	}
	card := a.card(book)
	if card.Author != "作者一、作者二" {
		t.Fatalf("多作者应拼在一起，得到 %q", card.Author)
	}
	if card.Source != "甲书.txt" {
		t.Fatalf("来源应只留文件名，得到 %q", card.Source)
	}
	if card.Format != "txt" {
		t.Fatalf("SourceFormat 为空应回落到 txt，得到 %q", card.Format)
	}
	if card.Chapters != 3 || card.Words != 900 || card.NoiseRatio != 0.1 {
		t.Fatalf("书卡字段没按解析产物填充: %+v", card)
	}
	if len(card.Samples) != 2 {
		t.Fatalf("空章应被跳过，样章 = %v", card.Samples)
	}
}

func TestSampleChaptersCaps(t *testing.T) {
	book := &model.ParsedBook{Chapters: []model.ParsedChapter{
		{Content: "一"}, {Content: "二"}, {Content: "三"}, {Content: "四"},
	}}
	if got := sampleChapters(book); len(got) != aiSampleChapters {
		t.Fatalf("样章应截断到 %d 段，得到 %d", aiSampleChapters, len(got))
	}
}

func TestJudgeEnforcesCallLimit(t *testing.T) {
	cur := &fakeCurator{want: []ai.Curation{{Decision: ai.DecisionImport}, {Decision: ai.DecisionImport}}}
	a := newAICurationForTest(cur, 1)
	if _, err := a.judge(context.Background(), &model.ParsedBook{Title: "甲"}); err != nil {
		t.Fatalf("第 1 次调用应放行: %v", err)
	}
	if _, err := a.judge(context.Background(), &model.ParsedBook{Title: "乙"}); err == nil {
		t.Fatal("超过 --ai-limit 应拒绝调用（fail-closed）")
	}
	if len(a.records) != 1 {
		t.Fatalf("被上限挡下的调用不该记进报告，records = %+v", a.records)
	}
}

func TestJudgeRecordsDecision(t *testing.T) {
	cur := &fakeCurator{want: []ai.Curation{{Decision: ai.DecisionSkip, Score: 12, Reasons: []string{"残卷"}}}}
	a := newAICurationForTest(cur, 0)
	v, err := a.judge(context.Background(), &model.ParsedBook{Title: "甲"})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if v.Import {
		t.Fatal("skip 应判为不入库")
	}
	if len(a.records) != 1 || a.records[0].Score != 12 || a.records[0].Decision != ai.DecisionSkip {
		t.Fatalf("评审记录 = %+v", a.records)
	}
}

// newAICurationForTest 用替身装配一个判定器（跳过 ListCategories 的库依赖）。
func newAICurationForTest(cur ai.Curator, limit int) *aiCuration {
	return &aiCuration{curator: cur, limit: limit}
}
