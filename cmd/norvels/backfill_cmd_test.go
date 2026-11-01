package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件是 `norvels backfill-fingerprints` 的端到端证据：造 3 本书 → 清空指纹 →
// 跑命令 → 断言补齐 → 再跑一遍断言「无需回填」（幂等，不重复插入）。

// seedBackfillBooks 建一个已迁移的库并写入 n 本内容互不相同的合成书，返回库路径与书 id。
func seedBackfillBooks(t *testing.T, n int) (string, []int64) {
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
			Title: fmt.Sprintf("回填样例%d", i), Author: "示例作者",
			SourcePath: fmt.Sprintf("/books/backfill-%d.txt", i), SourceName: "backfill.txt",
			Encoding: "utf-8", SHA256: strings.Repeat(fmt.Sprintf("%c", 'a'+i), 64),
		}
		for j := 0; j < 4; j++ {
			book.Chapters = append(book.Chapters, model.ParsedChapter{
				Idx: j, VolumeIdx: -1, Title: fmt.Sprintf("第%d章", j+1),
				Content: fmt.Sprintf("第%d本书第%d章：梅宁城的风从城楼吹过，雪线压得很低。", i, j),
			})
		}
		res, err := st.SaveBook(ctx, book, store.SaveOptions{})
		if err != nil {
			t.Fatalf("SaveBook: %v", err)
		}
		ids = append(ids, res.BookID)
	}
	// 模拟 0019 刚上线：表在、指纹一条都没有。
	if _, err := st.ContentDB().ExecContext(ctx, "DELETE FROM chapter_fingerprints"); err != nil {
		t.Fatalf("清空指纹: %v", err)
	}
	return dbPath, ids
}

// fingerprintRowCount 重新打开库并数指纹行数（不依赖被测代码的句柄）。
func fingerprintRowCount(t *testing.T, dbPath string) int64 {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("重新打开库: %v", err)
	}
	defer st.Close() //nolint:errcheck // 测试清理
	var n int64
	if err := st.ContentDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM chapter_fingerprints").Scan(&n); err != nil {
		t.Fatalf("数指纹行: %v", err)
	}
	return n
}

// TestBackfillFingerprintsCommand 3 本书回填 12 条指纹，重跑整本跳过。
func TestBackfillFingerprintsCommand(t *testing.T) {
	dbPath, ids := seedBackfillBooks(t, 3)
	if len(ids) != 3 {
		t.Fatalf("样例书 = %d 本，期望 3", len(ids))
	}
	if got := fingerprintRowCount(t, dbPath); got != 0 {
		t.Fatalf("清空后指纹行 = %d，期望 0", got)
	}

	var runErr error
	out := captureStdout(t, func() { runErr = cmdBackfillFingerprints([]string{"--db", dbPath, "--batch", "1"}) })
	if runErr != nil {
		t.Fatalf("回填命令: %v\n输出:\n%s", runErr, out)
	}
	for _, want := range []string{"回填完成", "扫描 3 本", "回填 3", "写入指纹 12 行"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
	if got := fingerprintRowCount(t, dbPath); got != 12 {
		t.Errorf("回填后指纹行 = %d，期望 12", got)
	}

	// 再跑一遍：幂等，整本跳过、不重复插入。
	out = captureStdout(t, func() { runErr = cmdBackfillFingerprints([]string{"--db", dbPath}) })
	if runErr != nil {
		t.Fatalf("第二次回填: %v\n输出:\n%s", runErr, out)
	}
	if !strings.Contains(out, "无需回填") {
		t.Errorf("第二次输出应说明无需回填：\n%s", out)
	}
	if got := fingerprintRowCount(t, dbPath); got != 12 {
		t.Errorf("重跑后指纹行 = %d，期望仍是 12", got)
	}
}

// TestPrintBackfillReportBadChapters 有坏章节时提示怎么定位（坏块不拦整批，但必须可见）。
func TestPrintBackfillReportBadChapters(t *testing.T) {
	out := captureStdout(t, func() {
		printBackfillReport(store.FingerprintBackfillReport{Books: 3, Filled: 2, BadChapters: 4})
	})
	if !containsStr(out, "坏章节 4 章") || !containsStr(out, "check-content") {
		t.Errorf("坏章节提示不完整：\n%s", out)
	}
}

// TestBackfillFingerprintsCommandEmptyDB 空库（0 本书）不报错，输出「无需回填」。
//
// 与其它子命令同一套打开语义（缺文件就建空库），所以这里断言的是输出文案而不是报错：
// 打错路径的人会看到「扫描 0 本」，而不是一个看不出来的空库。
func TestBackfillFingerprintsCommandEmptyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	var runErr error
	out := captureStdout(t, func() { runErr = cmdBackfillFingerprints([]string{"--db", dbPath}) })
	if runErr != nil {
		t.Fatalf("空库回填应成功，实际 err=%v", runErr)
	}
	if !containsStr(out, "无需回填") || !containsStr(out, "扫描 0 本") {
		t.Errorf("空库输出不明确：\n%s", out)
	}
}
