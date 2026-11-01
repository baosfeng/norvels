package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/parser"
	"norvels/internal/store"
)

// 本文件是 `norvels backfill-quality` 的端到端证据：
//   - 清空质量列 → 跑命令 → 覆盖率 100% → 重跑「无需回填」（幂等）；
//   - 回填出来的分与解析期口径在乱码 / 结构 / 重复三维一致（防两套公式漂移）。
//
// 语料全是造出来的（虚构书名 + 合成正文），不碰真实语料与真实书名。

// seedQualityBooks 建一个已迁移的库并写入 n 本合成书，返回库路径与书 id。
func seedQualityBooks(t *testing.T, n int) (string, []int64) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "norvels.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close() //nolint:errcheck // 测试清理
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		book := &model.ParsedBook{
			Title: fmt.Sprintf("质量样例%d", i), Author: "示例作者",
			SourcePath: fmt.Sprintf("/books/quality-%d.txt", i), SourceName: "quality.txt",
			Encoding: "utf-8", SHA256: strings.Repeat(fmt.Sprintf("%c", 'a'+i), 64),
		}
		for j := 0; j < 4; j++ {
			book.Chapters = append(book.Chapters, model.ParsedChapter{
				Idx: j, VolumeIdx: -1, Title: fmt.Sprintf("第%d章", j+1),
				Content: fmt.Sprintf("第%d本书第%d章：临川渡的雾从江面升起，艄公把竹篙插进水里。", i, j),
			})
		}
		res, err := st.SaveBook(ctx, book, store.SaveOptions{})
		if err != nil {
			t.Fatalf("SaveBook: %v", err)
		}
		ids = append(ids, res.BookID)
	}
	clearQualityColumns(t, dbPath)
	return dbPath, ids
}

// clearQualityColumns 把七列清空，模拟 0020 之前入的老库。
func clearQualityColumns(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("重新打开库: %v", err)
	}
	defer st.Close() //nolint:errcheck // 测试清理
	if _, err := st.DB().ExecContext(ctx,
		"UPDATE books SET garbage_ratio = 0, noise_ratio = 0, structure_score = 0, duplicate_ratio = 0, "+
			"quality_chapters = 0, quality_chars = 0, quality_at = ''"); err != nil {
		t.Fatalf("清空质量列: %v", err)
	}
}

// qualityCoverageOf 重新打开库并数「已算质量分」的书数（不依赖被测代码的句柄）。
func qualityCoverageOf(t *testing.T, dbPath string) (books, computed int) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("重新打开库: %v", err)
	}
	defer st.Close() //nolint:errcheck // 测试清理
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN quality_chapters > 0 AND quality_at <> '' THEN 1 ELSE 0 END), 0) "+
			"FROM books").Scan(&books, &computed); err != nil {
		t.Fatalf("数覆盖率: %v", err)
	}
	return books, computed
}

// TestBackfillQualityCommand 3 本书回填质量分，覆盖率 100%，重跑整本跳过。
func TestBackfillQualityCommand(t *testing.T) {
	dbPath, ids := seedQualityBooks(t, 3)
	if len(ids) != 3 {
		t.Fatalf("样例书 = %d 本，期望 3", len(ids))
	}
	if _, computed := qualityCoverageOf(t, dbPath); computed != 0 {
		t.Fatalf("清空后已算质量分的书 = %d，期望 0", computed)
	}

	var runErr error
	out := captureStdout(t, func() { runErr = cmdBackfillQuality([]string{"--db", dbPath, "--batch", "1"}) })
	if runErr != nil {
		t.Fatalf("回填命令: %v\n输出:\n%s", runErr, out)
	}
	for _, want := range []string{"回填完成", "扫描 3 本", "回填 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
	books, computed := qualityCoverageOf(t, dbPath)
	if books != 3 || computed != 3 {
		t.Fatalf("回填后覆盖率 = %d/%d，期望 3/3（100%%）", computed, books)
	}

	// 再跑一遍：幂等，整本跳过。
	out = captureStdout(t, func() { runErr = cmdBackfillQuality([]string{"--db", dbPath}) })
	if runErr != nil {
		t.Fatalf("第二次回填: %v\n输出:\n%s", runErr, out)
	}
	if !strings.Contains(out, "无需回填") {
		t.Errorf("第二次输出应说明无需回填：\n%s", out)
	}
}

// TestBackfillQualityCommandEmptyDB 空库不报错，输出「无需回填」。
func TestBackfillQualityCommandEmptyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	var runErr error
	out := captureStdout(t, func() { runErr = cmdBackfillQuality([]string{"--db", dbPath}) })
	if runErr != nil {
		t.Fatalf("空库回填应成功，实际 err=%v", runErr)
	}
	if !strings.Contains(out, "无需回填") || !strings.Contains(out, "扫描 0 本") {
		t.Errorf("空库输出不明确：\n%s", out)
	}
}

// TestBackfillQualityMatchesParser 回填口径与解析期口径一致（乱码 / 结构 / 重复三维）。
//
// 这是「两套公式不许漂移」的守门测试：同一份脏文本先走解析链路（导入时算的分），
// 再清空七列走回填（从库里正文重算的分），逐项对比。噪声维度不在对比范围：
// 回填只能统计残留噪声（库里正文已清洗过），这是文档写明的唯一差别。
func TestBackfillQualityMatchesParser(t *testing.T) {
	parsed, backfilled := parseThenBackfill(t)
	assertSameQuality(t, parsed, backfilled)
}

// parseThenBackfill 走一遍「解析 → 入库 → 清空七列 → 跑回填命令」，返回两个阶段的质量分。
func parseThenBackfill(t *testing.T) (store.BookQuality, store.BookQuality) {
	t.Helper()
	ctx := context.Background()
	book := dirtyParsedBook(t)
	dbPath := filepath.Join(t.TempDir(), "norvels.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	res, err := st.SaveBook(ctx, book, store.SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	parsed, err := st.BookQuality(ctx, res.BookID)
	if err != nil {
		t.Fatalf("BookQuality: %v", err)
	}
	_ = st.Close() //nolint:errcheck // 换一个句柄跑命令

	clearQualityColumns(t, dbPath)
	var runErr error
	out := captureStdout(t, func() { runErr = cmdBackfillQuality([]string{"--db", dbPath}) })
	if runErr != nil {
		t.Fatalf("回填命令: %v\n输出:\n%s", runErr, out)
	}

	st2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("重新打开库: %v", err)
	}
	defer st2.Close() //nolint:errcheck // 测试清理
	backfilled, err := st2.BookQuality(ctx, res.BookID)
	if err != nil {
		t.Fatalf("BookQuality(回填后): %v", err)
	}
	return parsed, backfilled
}

// dirtyParsedBook 用真实解析链路解析脏样例，并补齐来源字段（等价于 importer.fillBookSource）。
func dirtyParsedBook(t *testing.T) *model.ParsedBook {
	t.Helper()
	book, err := parser.New(parser.DefaultOptions()).ParseBytes("脏样例.txt", []byte(dirtySampleText()))
	if err != nil {
		t.Fatalf("解析: %v", err)
	}
	book.SourcePath = "/books/dirty.txt"
	book.SourceName = "脏样例.txt"
	book.SHA256 = strings.Repeat("d", 64)
	if book.Quality.GarbageRatio <= 0 || book.Quality.DuplicateRatio <= 0 {
		t.Fatalf("解析期质量分 = %+v，期望乱码率与重复率都 > 0（语料要真的脏）", book.Quality)
	}
	return book
}

// assertSameQuality 对比解析期与回填期的质量分（噪声维度除外，见函数注释）。
func assertSameQuality(t *testing.T, parsed, backfilled store.BookQuality) {
	t.Helper()
	if !backfilled.Computed() {
		t.Fatalf("回填后质量分仍未计算：%+v", backfilled)
	}
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"乱码率", backfilled.GarbageRatio, parsed.GarbageRatio},
		{"结构分", backfilled.StructureScore, parsed.StructureScore},
		{"内部重复率", backfilled.DuplicateRatio, parsed.DuplicateRatio},
	} {
		if diff := c.got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s：回填 %v，解析期 %v（两套公式漂移了）", c.name, c.got, c.want)
		}
	}
	if backfilled.Chapters != parsed.Chapters || backfilled.Chars != parsed.Chars {
		t.Errorf("章数/字符数：回填 %d/%d，解析期 %d/%d",
			backfilled.Chapters, backfilled.Chars, parsed.Chapters, parsed.Chars)
	}
	if backfilled.NoiseRatio != 0 {
		t.Errorf("回填噪声率 = %v，期望 0（库里正文已清洗过，只能统计残留噪声）", backfilled.NoiseRatio)
	}
}

// dirtySampleText 造一份「有广告行、有乱码、有重复段落、章长不均」的合成文本。
func dirtySampleText() string {
	var b strings.Builder
	b.WriteString("书名：脏样例\n作者：示例作者\n\n")
	para := "临川渡的雾从江面升起，艄公把竹篙插进水里，岸边的灯一盏一盏亮起来。\n"
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "第%d章 风雪%d\n", i+1, i)
		body := para
		if i == 3 {
			body += "这一章有乱码：\uFFFD\uFFFD\uFFFD\uFFFD\n"
		}
		if i < 3 {
			body = strings.Repeat(para, 3) // 前三章更长，制造离群
		}
		b.WriteString(body)
		b.WriteString("本作品来自互联网，请支持正版。\n\n")
	}
	return b.String()
}
