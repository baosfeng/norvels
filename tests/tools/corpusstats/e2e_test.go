package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件是「拆库之后 corpusstats 还能不能用」的端到端证据：库由 store 真实迁移到 0018
// （块表只在正文库），不是手搓的两张表。
//
// 只有**测试**通过 store 造库；工具本体仍然只依赖标准库——门禁要求 tests/ 下的工具包
// 不 import internal（见 tests/tools/gatedeps 的 checkDirection），测试造 fixture 不改变这一点。

// TestRunOnMigratedSplitLibrary 迁移到 0018 的库上跑通 -all：
// 书数来自主库、块与字符频次来自正文库。
func TestRunOnMigratedSplitLibrary(t *testing.T) {
	mainPath := buildMigratedLibrary(t, context.Background())
	// 生产形态：主库没有块表，正文库有——正是工具要读的那份。
	assertSplitLayout(t, mainPath)

	out := filepath.Join(t.TempDir(), "out")
	if err := run([]string{"-db", mainPath, "-out", out, "-all", "-workers", "2"}); err != nil {
		t.Fatalf("在拆库上跑 corpusstats 失败: %v", err)
	}
	assertSplitStats(t, readStats(t, out))
}

// buildMigratedLibrary 用 store 造一个 0018 之后的真实库（2 本书 / 9 章），返回主库路径。
func buildMigratedLibrary(t *testing.T, ctx context.Context) string {
	t.Helper()
	mainPath := filepath.Join(t.TempDir(), "norvels.db")
	st, err := store.Open(ctx, mainPath)
	if err != nil {
		t.Fatalf("开库: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("迁移到最新版本: %v", err)
	}
	books := []*model.ParsedBook{
		synthBook("长夜书简", "/path/to/txt/a.txt", "a", 6),
		synthBook("荒渡灯影", "/path/to/txt/b.txt", "b", 3),
	}
	for _, b := range books {
		if _, err := st.SaveBook(ctx, b, store.SaveOptions{}); err != nil {
			t.Fatalf("SaveBook %q: %v", b.Title, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关库: %v", err)
	}
	return mainPath
}

// assertSplitStats 校验拆库上的统计口径：书数来自主库、块与字符频次来自正文库，
// 且 0017 起的 0x03 码本块必须被正确解码（否则统计会直接报错或直方图是 unknown）。
func assertSplitStats(t *testing.T, stats statsJSON) {
	t.Helper()
	if stats.Books != 2 || stats.Blocks == 0 || stats.TotalRune == 0 || stats.RawBytes == 0 || stats.Mode != "all" {
		t.Fatalf("统计口径为空或模式不对: %+v", stats)
	}
	if stats.AlgoHistogram[algoName(algoCodebook)] != stats.Blocks {
		t.Fatalf("算法直方图不对（应全部是码本块）: %v / blocks=%d", stats.AlgoHistogram, stats.Blocks)
	}
}

// synthBook 造一本合成书：n 章，每章一段可压缩的中文正文。
func synthBook(title, path, salt string, n int) *model.ParsedBook {
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
		text := "华夏国临川市，午后的秋日，微风拂过城楼。他站在城楼上，望着远方的梅宁城与风雪。"
		b.Chapters = append(b.Chapters, model.ParsedChapter{
			Idx: i, VolumeIdx: 0, Title: "第" + string(rune('一'+i)) + "章",
			Content: text, WordCount: len([]rune(text)),
		})
	}
	return b
}

// assertSplitLayout 断言拆库形态：主库没有 chapter_blocks，正文库有。
func assertSplitLayout(t *testing.T, mainPath string) {
	t.Helper()
	main, err := openReadOnly(mainPath)
	if err != nil {
		t.Fatalf("只读打开主库: %v", err)
	}
	defer func() { _ = main.Close() }()
	var n int
	if err := main.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'chapter_blocks'").Scan(&n); err != nil {
		t.Fatalf("查主库表: %v", err)
	}
	if n != 0 {
		t.Fatalf("主库仍存在 chapter_blocks（迁移 0018 未生效）")
	}
	content, err := openReadOnly(contentDBPath(mainPath))
	if err != nil {
		t.Fatalf("只读打开正文库: %v", err)
	}
	defer func() { _ = content.Close() }()
	var blocks int
	if err := content.QueryRow("SELECT COUNT(*) FROM chapter_blocks").Scan(&blocks); err != nil {
		t.Fatalf("查正文库块表: %v", err)
	}
	if blocks == 0 {
		t.Fatal("正文库没有块")
	}
}
