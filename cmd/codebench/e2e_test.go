package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件是「拆库之后工具还能不能用」的端到端证据：库不是手搓的两张表，
// 而是 store 真实迁移到 0018 之后的状态（块表只存在于正文库）。
//
// 手搓 fixture（db_test.go）锁的是 SQL 口径；这里锁的是「生产库形态」。

// buildSplitLibrary 用 store 造一个 0018 之后的真实库，返回主库路径与各书 id。
func buildSplitLibrary(t *testing.T, ctx context.Context) (string, []int64) {
	t.Helper()
	mainPath := filepath.Join(t.TempDir(), "norvels.db")
	st, err := store.Open(ctx, mainPath)
	if err != nil {
		t.Fatalf("开库: %v", err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("迁移到最新版本: %v", err)
	}
	books := []*model.ParsedBook{
		parsedBook("长夜书简", "/path/to/txt/a.txt", "a", 40),
		parsedBook("荒渡灯影", "/path/to/txt/b.txt", "b", 12),
	}
	ids := make([]int64, 0, len(books))
	for _, b := range books {
		res, err := st.SaveBook(ctx, b, store.SaveOptions{})
		if err != nil {
			t.Fatalf("SaveBook %q: %v", b.Title, err)
		}
		ids = append(ids, res.BookID)
	}
	return mainPath, ids
}

// parsedBook 造一本合成书：n 章，每章一段可压缩的中文正文。
func parsedBook(title, path, salt string, n int) *model.ParsedBook {
	b := &model.ParsedBook{
		Title:      title,
		Author:     "青川客",
		SourcePath: path,
		SourceName: filepath.Base(path),
		Encoding:   "utf-8",
		FileSize:   4096,
		SHA256:     strings.Repeat(salt, 64),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
	}
	for i := 0; i < n; i++ {
		content := fmt.Sprintf("第%d章 风起\n华夏国临川市，午后的秋日，微风拂过城楼。他站在城楼上，望着远方的梅宁城与风雪。", i+1)
		b.Chapters = append(b.Chapters, model.ParsedChapter{
			Idx: i, VolumeIdx: 0, Title: fmt.Sprintf("第%d章 风起", i+1),
			Content: content, WordCount: len([]rune(content)),
		})
	}
	return b
}

// TestRunOnMigratedSplitLibrary 迁移到 0018 的库上跑通 codebench：
// 块从正文库读、章节尺寸从主库读、库内 BLOB 对账来自正文库。
func TestRunOnMigratedSplitLibrary(t *testing.T) {
	ctx := context.Background()
	mainPath, ids := buildSplitLibrary(t, ctx)
	assertBlockTableLocation(t, mainPath)
	contentPath := store.ContentDBPath(mainPath)

	out := filepath.Join(t.TempDir(), "cb.md")
	if err := run(mainPath, "", 2, out, ids[len(ids)-1]); err != nil {
		t.Fatalf("在拆库上跑 codebench 失败: %v", err)
	}
	assertSplitBenchResult(t, readResult(t, out), contentPath)
}

// assertSplitBenchResult 校验拆库上的跑分产物：正文库路径、全库口径、五组全过、最大书往返。
func assertSplitBenchResult(t *testing.T, res Result, contentPath string) {
	t.Helper()
	if res.ContentDBPath != contentPath {
		t.Errorf("正文库路径 = %q，期望 %q", res.ContentDBPath, contentPath)
	}
	// 52 章按 8 MB 目标切块：块数必然大于 0 且不超过章数（一块至少覆盖一章）。
	if blocks := res.Library.Blocks; blocks <= 0 || blocks > 52 || res.Library.Books != 2 {
		t.Fatalf("全库口径不合理：%+v", res.Library)
	}
	assertAllGroupsPassed(t, res)
	if res.Largest == nil || !res.Largest.RoundTrip {
		t.Fatalf("最大书整本往返未通过：%+v", res.Largest)
	}
}

// TestRunOnSplitLibrarySampleZero -sample 0 只出全库口径（不抽样），
// 这正是 ADR-0007 里「快速校准全库比值」的用法。
func TestRunOnSplitLibrarySampleZero(t *testing.T) {
	ctx := context.Background()
	mainPath, _ := buildSplitLibrary(t, ctx)
	out := filepath.Join(t.TempDir(), "cb.md")
	if err := run(mainPath, "", 0, out, 0); err != nil {
		t.Fatalf("只做全库口径失败: %v", err)
	}
	res := readResult(t, out)
	if len(res.Groups) != 0 || res.Largest != nil {
		t.Fatalf("-sample 0 不该出方案表或最大书：%+v", res)
	}
	if res.Library.RawBytes <= 0 || res.Library.PackedBytes <= 0 {
		t.Fatalf("全库体积口径为空：%+v", res.Library)
	}
}

// assertBlockTableLocation 断言拆库形态：主库无 chapter_blocks，正文库有。
func assertBlockTableLocation(t *testing.T, mainPath string) {
	t.Helper()
	db, err := openReadOnly(mainPath)
	if err != nil {
		t.Fatalf("只读打开主库: %v", err)
	}
	defer db.Close() //nolint:errcheck // 只读句柄
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'chapter_blocks'").Scan(&n); err != nil {
		t.Fatalf("查主库表: %v", err)
	}
	if n != 0 {
		t.Fatalf("主库仍存在 chapter_blocks（迁移 0018 未生效）")
	}
	cdb, err := openReadOnly(store.ContentDBPath(mainPath))
	if err != nil {
		t.Fatalf("只读打开正文库: %v", err)
	}
	defer cdb.Close() //nolint:errcheck // 只读句柄
	var blocks int
	if err := cdb.QueryRow("SELECT COUNT(*) FROM chapter_blocks").Scan(&blocks); err != nil {
		t.Fatalf("查正文库块表: %v", err)
	}
	if blocks == 0 {
		t.Fatal("正文库没有块")
	}
}

// readResult 读回 -out 同名 .json。
func readResult(t *testing.T, out string) Result {
	t.Helper()
	blob, err := os.ReadFile(strings.TrimSuffix(out, filepath.Ext(out)) + ".json")
	if err != nil {
		t.Fatalf("结果 JSON 未落盘: %v", err)
	}
	var res Result
	if err := json.Unmarshal(blob, &res); err != nil {
		t.Fatalf("结果 JSON 不可解析: %v", err)
	}
	return res
}
