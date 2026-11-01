package importer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/store"
)

// 本文件是内容去重（设计 §4）在导入链路上的端到端证据：几本合成书走一遍真实 Run，
// 断言「纯重复被跳过」「改 10% 放行并标记相关」「新书正常」以及 force 跳过判定。
// 语料全是造出来的（虚构书名 + 合成正文），不碰真实语料与真实书名。

const dedupeChapterCount = 10

// dedupeBody 生成第 i 章正文：每章内容不同、长度接近，且不含章节/卷标记
// （含「第 N 节」这类行会被切章器当成新章节）。
func dedupeBody(i int) string {
	return strings.Repeat(fmt.Sprintf("梅宁城第%d次下雪，少年按住剑柄，远处的雪线压得很低。\n", i), 24)
}

// dedupeNovel 生成一本可导入的合成小说：changed 为 1 基章号时把那一章换成等长的全新正文。
func dedupeNovel(title string, changed int) string {
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：示例作者\n\n")
	for i := 0; i < dedupeChapterCount; i++ {
		b.WriteString(fmt.Sprintf("第%d章 风雪%d\n", i+1, i))
		body := dedupeBody(i)
		if changed > 0 && i == changed-1 {
			body = strings.Repeat("这一章被彻底改写：孤舟泊岸，灯火在雨里摇晃，另有一个结局。\n", 24)
		}
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// noiseUp 只改排版（每行之间多插空行）：字节变了、章级内容不变。
// 用空行而不是加空格，避免把「书名：」这类元信息行弄成认不出来的形状。
func noiseUp(text string) string {
	return strings.Join(strings.Split(text, "\n"), "\n\n")
}

// importDedupe 把一段文本写进独立目录整批导入，返回结果与任务里的文件级记录。
func importDedupe(t *testing.T, ctx context.Context, st *store.Store, name, text string, force bool) (*Result, []store.ImportJobFile) {
	t.Helper()
	dir := t.TempDir()
	writeTxt(t, dir, name, text)
	opt := opts(dir)
	opt.Force = force
	res, err := Run(ctx, st, opt, nil)
	if err != nil {
		t.Fatalf("Run(%s): %v", name, err)
	}
	if res.Job == nil {
		t.Fatalf("Run(%s) 没有建任务记录", name)
	}
	files, err := st.ListImportJobFiles(ctx, res.Job.ID, "", 10)
	if err != nil {
		t.Fatalf("ListImportJobFiles: %v", err)
	}
	return res, files.Files
}

// onlyJobFile 取任务里唯一的那条文件记录。
func onlyJobFile(t *testing.T, files []store.ImportJobFile) store.ImportJobFile {
	t.Helper()
	if len(files) != 1 {
		t.Fatalf("文件级记录 = %d 条，期望 1 条：%+v", len(files), files)
	}
	return files[0]
}

// assertFileState 断言文件级终态与备注。
func assertFileState(t *testing.T, f store.ImportJobFile, status, reasonPart string) {
	t.Helper()
	if f.Status != status {
		t.Fatalf("文件终态 = %q，期望 %q（备注 %q）", f.Status, status, f.Reason)
	}
	if reasonPart != "" && !strings.Contains(f.Reason, reasonPart) {
		t.Fatalf("备注 = %q，期望包含 %q", f.Reason, reasonPart)
	}
}

// dedupeFixture 导入原书并返回库、上下文与原文（每例独立环境，互不干扰）。
func dedupeFixture(t *testing.T) (*store.Store, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	original := dedupeNovel("甲书", 0)
	res, files := importDedupe(t, ctx, st, "甲书.txt", original, false)
	if len(res.Imported) != 1 || res.Skipped != 0 {
		t.Fatalf("原书导入汇总 = %+v，期望 1 本入库", res)
	}
	assertFileState(t, onlyJobFile(t, files), store.ImportFileStatusImported, "")
	return st, ctx, original
}

// TestImportDedupeSkipsDuplicate 纯重复（只有排版差异、SHA 不同）：按 Skipped 并给出书名。
func TestImportDedupeSkipsDuplicate(t *testing.T) {
	st, ctx, original := dedupeFixture(t)
	res, files := importDedupe(t, ctx, st, "甲书-排版版.txt", noiseUp(original), false)
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("导入汇总 = %+v，期望 1 本按重复跳过", res)
	}
	f := onlyJobFile(t, files)
	assertFileState(t, f, store.ImportFileStatusSkipped, "疑似重复")
	for _, want := range []string{"100%", "《甲书》"} {
		if !strings.Contains(f.Reason, want) {
			t.Errorf("备注 = %q，期望包含 %q", f.Reason, want)
		}
	}
}

// TestImportDedupeMarksRelated 改 10%：放行入库，文件备注写「疑似相关」。
func TestImportDedupeMarksRelated(t *testing.T) {
	st, ctx, _ := dedupeFixture(t)
	res, files := importDedupe(t, ctx, st, "甲书-改版.txt", dedupeNovel("甲书改", 5), false)
	if len(res.Imported) != 1 || res.Skipped != 0 {
		t.Fatalf("导入汇总 = %+v，期望 1 本入库", res)
	}
	f := onlyJobFile(t, files)
	assertFileState(t, f, store.ImportFileStatusImported, "疑似相关")
	if strings.Contains(f.Reason, "疑似重复") {
		t.Errorf("备注 = %q，90%% 命中不该判成重复", f.Reason)
	}
}

// TestImportDedupeImportsNewBook 全新内容：正常入库、无备注。
func TestImportDedupeImportsNewBook(t *testing.T) {
	st, ctx, _ := dedupeFixture(t)
	fresh := strings.ReplaceAll(dedupeNovel("乙书", 0), "梅宁城", "临川渡")
	res, files := importDedupe(t, ctx, st, "乙书.txt", fresh, false)
	if len(res.Imported) != 1 || res.Skipped != 0 {
		t.Fatalf("导入汇总 = %+v，期望 1 本入库", res)
	}
	assertFileState(t, onlyJobFile(t, files), store.ImportFileStatusImported, "")
}

// TestImportDedupeForceSkipsJudge force 一律跳过判定：连完全重复的也照常导入。
func TestImportDedupeForceSkipsJudge(t *testing.T) {
	st, ctx, original := dedupeFixture(t)
	res, files := importDedupe(t, ctx, st, "甲书-force.txt", original, true)
	if len(res.Imported) != 1 || res.Skipped != 0 {
		t.Fatalf("导入汇总 = %+v，强制导入应按入库计", res)
	}
	f := onlyJobFile(t, files)
	assertFileState(t, f, store.ImportFileStatusImported, "")
	if strings.Contains(f.Reason, "疑似") {
		t.Errorf("force 导入的备注 = %q，不该判重", f.Reason)
	}
}

// TestDuplicateReasonText 备注文案与阈值回落的口径。
func TestDuplicateReasonText(t *testing.T) {
	rep := store.DedupeReport{HitRate: 1, DuplicateBookTitle: "甲书"}
	if got, want := duplicateReason(rep), "疑似重复：库内已有 100% 相同内容（《甲书》可对照）"; got != want {
		t.Errorf("重复备注 = %q，期望 %q", got, want)
	}
	rep.DuplicateBookTitle = ""
	if got := duplicateReason(rep); !strings.Contains(got, "《书名未知》") {
		t.Errorf("缺书名时备注 = %q，期望写「书名未知」", got)
	}
	if got, want := relatedNote(store.DedupeReport{HitRate: 0.4321}), "疑似相关：43%"; got != want {
		t.Errorf("相关备注 = %q，期望 %q", got, want)
	}
}

// TestJoinReasonAndThreshold 备注拼接与阈值回落。
func TestJoinReasonAndThreshold(t *testing.T) {
	cases := []struct{ first, second, want string }{
		{"库中已存在同一本书", "", "库中已存在同一本书"},
		{"", "疑似相关：90%", "疑似相关：90%"},
		{"库中已存在同一本书", "疑似相关：90%", "库中已存在同一本书；疑似相关：90%"},
	}
	for _, tc := range cases {
		if got := joinReason(tc.first, tc.second); got != tc.want {
			t.Errorf("joinReason(%q,%q) = %q，期望 %q", tc.first, tc.second, got, tc.want)
		}
	}
	if got := (Options{}).dedupeThreshold(); got != store.DefaultDedupeThreshold {
		t.Errorf("未配置阈值 = %v，期望缺省 %v", got, store.DefaultDedupeThreshold)
	}
	if got := (Options{DedupeThreshold: 0.5}).dedupeThreshold(); got != 0.5 {
		t.Errorf("配置阈值 = %v，期望 0.5", got)
	}
	if got := (Options{DedupeThreshold: 1.5}).dedupeThreshold(); got != store.DefaultDedupeThreshold {
		t.Errorf("越界阈值应回落缺省，实际 %v", got)
	}
}
