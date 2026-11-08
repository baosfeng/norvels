package main

import (
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
	"norvels/internal/store"
)

// seedClassifiableBooks 造 n 本「书名能被本地词表分类」的待补书，
// 用来端到端跑 apply 路径（不需要任何在线书源）。
func seedClassifiableBooks(t *testing.T, st *store.Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		book := &model.ParsedBook{
			Title:      "修仙示例录第" + string(rune('一'+i)) + "卷",
			Author:     "佚名",
			SourcePath: "/books/demo-cls-" + string(rune('a'+i)) + ".txt",
			SourceName: "demo-cls.txt",
			Encoding:   "utf-8", FileSize: 1024,
			SHA256:    strings.Repeat(string(rune('a'+i)), 64),
			WordCount: 30,
			Chapters: []model.ParsedChapter{
				{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: "示例正文。", WordCount: 5},
			},
		}
		if _, err := st.SaveBook(t.Context(), book, store.SaveOptions{}); err != nil {
			t.Fatalf("种子书 %d: %v", i, err)
		}
	}
}

// TestRunEnrichApplyStreamsToDB 端到端跑 apply：分批落库、同一批次号、进度行与批次号都打印。
func TestRunEnrichApplyStreamsToDB(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	seedClassifiableBooks(t, st, 5)
	var progress strings.Builder
	sink := enrichSink(st, enrichOpts{apply: true})
	sink.Out = &progress
	batchID := sink.BatchID

	out := captureStdout(t, func() {
		err := runEnrichTo(t.Context(), st, enrichOpts{
			apply: true, fieldsFlag: "category", workers: 2, qps: 1, maxSources: 1,
			timeout: time.Second, minTitleSim: 0.86, flushEvery: 2,
		}, &sink)
		if err != nil {
			t.Fatalf("apply 不应报错: %v", err)
		}
	})

	rows, err := st.EnrichBatchChanges(t.Context(), batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("apply 后 enrich_changes 应有记录（批次 %s），实际 0 条；输出 %q", batchID, out)
	}
	if !strings.Contains(progress.String(), "进度：") {
		t.Fatalf("应打印进度行，实际 %q", progress.String())
	}
	if !strings.Contains(out, "批次号："+batchID) {
		t.Fatalf("应打印批次号 %q，实际 %q", batchID, out)
	}
}

// TestRunEnrichDryRunKeepsNoDetail dry-run 不落库也不留明细（几万本的明细没有消费者）。
func TestRunEnrichDryRunKeepsNoDetail(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	seedClassifiableBooks(t, st, 2)
	sink := enrichSink(st, enrichOpts{apply: false})
	if sink.Flush != nil || sink.BatchID != "" {
		t.Fatalf("dry-run 的 sink 不该带落库能力，实际 %+v", sink)
	}
}
