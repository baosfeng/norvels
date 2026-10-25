package store

import (
	"context"
	"os"
	"testing"
)

// TestRealDBImportUpdatesMetrics 在真实库副本上导入一本书，校验 store_metrics 自动更新且与精确值一致。
//
//	NORVELS_REAL_DB=/path/to/norvels.db go test ./internal/store/ -run TestRealDBImportUpdatesMetrics -v
func TestRealDBImportUpdatesMetrics(t *testing.T) {
	src := os.Getenv("NORVELS_REAL_DB")
	if src == "" {
		t.Skip("设置 NORVELS_REAL_DB")
	}
	ctx := context.Background()
	st := openRealCopy(t, ctx, src)
	defer st.Close()

	before := requireMetrics(t, ctx, st)
	// 导入一本新书（3 章），再校验聚合值随之增长。
	book := sampleBook("metrics 验证书", "验证作者", "/books/metrics-check.txt", repeat64("q"))
	if res, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil || res.Skipped {
		t.Fatalf("SaveBook = %+v, err=%v", res, err)
	}
	after := requireMetrics(t, ctx, st)
	if after.TextBytes <= before.TextBytes || after.ChapterWordSum <= before.ChapterWordSum {
		t.Errorf("导入后未增长：前 %+v，后 %+v", before, after)
	}
	assertMetricsExact(t, ctx, st, after)

	// 覆盖写入（Force）不应重复计数。
	if _, err := st.SaveBook(ctx, book, SaveOptions{Force: true}); err != nil {
		t.Fatalf("Force SaveBook: %v", err)
	}
	forced := requireMetrics(t, ctx, st)
	if forced != after {
		t.Errorf("Force 后重复计数：前 %+v，后 %+v", after, forced)
	}
}

// openRealCopy 复制真实库到临时目录后打开（破坏性测试不碰原始数据）。
func openRealCopy(t *testing.T, ctx context.Context, src string) *Store {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读源库: %v", err)
	}
	dst := t.TempDir() + "/copy.db"
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("写副本: %v", err)
	}
	st, err := Open(ctx, dst)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return st
}

// requireMetrics 读库级聚合值并要求成功。
func requireMetrics(t *testing.T, ctx context.Context, st *Store) metricsSnapshot {
	t.Helper()
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	return snap
}

// assertMetricsExact 校验增量累加值等于全表精确值（误差 0）。
func assertMetricsExact(t *testing.T, ctx context.Context, st *Store, got metricsSnapshot) {
	t.Helper()
	wantText, err := exactTextBytes(ctx, st)
	if err != nil {
		t.Fatalf("精确文本: %v", err)
	}
	wantWords, err := st.scalarInt64(ctx, chapterWordSumSQL)
	if err != nil {
		t.Fatalf("精确字数: %v", err)
	}
	if got.TextBytes != wantText {
		t.Errorf("增量 textBytes = %d，精确值 %d", got.TextBytes, wantText)
	}
	if got.ChapterWordSum != wantWords {
		t.Errorf("增量 words = %d，精确值 %d", got.ChapterWordSum, wantWords)
	}
}
