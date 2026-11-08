package main

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/crawl"
	"norvels/internal/enrich"
	"norvels/internal/store"
)

// fakeSearchEngine 返回固定命中，用来验证「决策 → 变更」的字段口径（不联网）。
type fakeSearchEngine struct{ hits []enrich.Hit }

func (f fakeSearchEngine) SearchBook(context.Context, string) (crawl.Result, error) {
	return crawl.Result{Hits: f.hits}, nil
}

func (fakeSearchEngine) Stats() crawl.Stats { return crawl.Stats{} }

// allFieldCandidate 是三个字段都待补的候选（占位作者、空简介、未分类）。
func allFieldCandidate() store.EnrichCandidate {
	return store.EnrichCandidate{BookID: 1, Title: "示例书", Author: "佚名"}
}

// allFieldHit 是同时带作者/简介/分类的命中；书名与候选完全一致。
func allFieldHit() enrich.Hit {
	return enrich.Hit{
		SourceID: "src-a", Priority: 1, Title: "示例书", Author: "作者甲",
		Intro: "这是一段足够长的示例简介，用来验证简介字段能够参与决策并写进变更清单。",
		Kind:  "玄幻/东方玄幻",
	}
}

// runFieldsLoop 用假引擎跑一次单书决策，返回报告。
func runFieldsLoop(t *testing.T, fields map[string]bool) enrichReport {
	t.Helper()
	st := openTempStore(t)
	t.Cleanup(func() { _ = st.Close() })
	o := enrichOpts{fields: fields, workers: 1, minTitleSim: 0.86}
	return runEnrichLoop(t.Context(), st, fakeSearchEngine{hits: []enrich.Hit{allFieldHit()}},
		[]store.EnrichCandidate{allFieldCandidate()}, o)
}

// requireChangeFields 断言变更清单的字段恰好是 want（顺序固定）。
func requireChangeFields(t *testing.T, changes []store.EnrichChange, want ...string) {
	t.Helper()
	if len(changes) != len(want) {
		t.Fatalf("变更 = %+v，期望字段 %v", changes, want)
	}
	for i, field := range want {
		if changes[i].Field != field {
			t.Fatalf("变更[%d].Field = %q，期望 %q（全部变更 %+v）", i, changes[i].Field, field, changes)
		}
	}
}

// requireNoSkipField 断言报告里没有被排除字段的跳过原因（如 intro:too_short）。
func requireNoSkipField(t *testing.T, skipped map[string]int, fields ...string) {
	t.Helper()
	for key := range skipped {
		for _, field := range fields {
			if strings.HasPrefix(key, field+":") {
				t.Errorf("未选中的字段 %s 不应出现在跳过原因里：%q", field, key)
			}
		}
	}
}

// TestEnrichFieldsAuthorOnly -fields author 时即使源同时给了简介和分类，也只变更作者。
func TestEnrichFieldsAuthorOnly(t *testing.T) {
	rep := runFieldsLoop(t, map[string]bool{enrich.FieldAuthor: true})
	requireChangeFields(t, rep.Changes, store.EnrichFieldAuthor)
	requireNoSkipField(t, rep.Skipped, enrich.FieldIntro, enrich.FieldCategory)
}

// TestEnrichFieldsAuthorIntro -fields author,intro 时两条变更都有，分类不参与。
func TestEnrichFieldsAuthorIntro(t *testing.T) {
	rep := runFieldsLoop(t, map[string]bool{enrich.FieldAuthor: true, enrich.FieldIntro: true})
	requireChangeFields(t, rep.Changes, store.EnrichFieldAuthor, store.EnrichFieldIntro)
	requireNoSkipField(t, rep.Skipped, enrich.FieldCategory)
}

// TestEnrichFieldsCategoryOnly -fields category 时只变更分类。
func TestEnrichFieldsCategoryOnly(t *testing.T) {
	rep := runFieldsLoop(t, map[string]bool{enrich.FieldCategory: true})
	requireChangeFields(t, rep.Changes, store.EnrichFieldCategory)
	requireNoSkipField(t, rep.Skipped, enrich.FieldAuthor, enrich.FieldIntro)
}

// TestEnrichFieldsDefaultAll 不传字段集合时保持旧行为：三个字段都判。
func TestEnrichFieldsDefaultAll(t *testing.T) {
	rep := runFieldsLoop(t, nil)
	requireChangeFields(t, rep.Changes,
		store.EnrichFieldAuthor, store.EnrichFieldIntro, store.EnrichFieldCategory)
}

// TestEnrichDecisionOptionsCarriesFields 决策参数必须带上字段集合（apply 路径的入口）。
func TestEnrichDecisionOptionsCarriesFields(t *testing.T) {
	fields := map[string]bool{enrich.FieldAuthor: true}
	opt := enrichDecisionOptions(enrichOpts{fields: fields, minTitleSim: 0.9})
	if !opt.Fields[enrich.FieldAuthor] || opt.Fields[enrich.FieldIntro] {
		t.Errorf("Fields = %v，期望只有 author", opt.Fields)
	}
	if opt.MinTitleSim != 0.9 {
		t.Errorf("MinTitleSim = %v，期望 0.9", opt.MinTitleSim)
	}
}

// TestPrepareEnrichSameFieldsForPlanAndApply --plan 与 --apply 必须用同一份字段集合挑候选。
func TestPrepareEnrichSameFieldsForPlanAndApply(t *testing.T) {
	st := openTempStore(t)
	defer st.Close() //nolint:errcheck // 测试清理
	seedCandidateBooks(t, st)
	fields := "author"
	planO, planC, err := prepareEnrich(t.Context(), st, enrichOpts{planOnly: true, fieldsFlag: fields})
	if err != nil {
		t.Fatalf("prepareEnrich(plan): %v", err)
	}
	applyO, applyC, err := prepareEnrich(t.Context(), st, enrichOpts{apply: true, fieldsFlag: fields})
	if err != nil {
		t.Fatalf("prepareEnrich(apply): %v", err)
	}
	if !planO.fields[enrich.FieldAuthor] || planO.fields[enrich.FieldIntro] || planO.fields[enrich.FieldCategory] {
		t.Errorf("plan fields = %v，期望只有 author", planO.fields)
	}
	if len(planC) != len(applyC) {
		t.Fatalf("plan/apply 候选数 = %d/%d，期望一致", len(planC), len(applyC))
	}
	for i := range planC {
		if planC[i].BookID != applyC[i].BookID {
			t.Fatalf("plan/apply 候选不一致：%+v vs %+v", planC, applyC)
		}
	}
	if len(applyO.fields) != 1 {
		t.Errorf("apply fields = %v，期望与 plan 一致（只有 author）", applyO.fields)
	}
}
