package importer

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/store"
)

// 合成合集与追更语料：书名/作者/正文全部虚构（隐私红线）。

// chaptersBlock 生成 n 章「第N章 标签」正文，标题稳定可复现。
func chaptersBlock(n int, tag string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("第")
		b.WriteString(itoa(i))
		b.WriteString("章 ")
		b.WriteString(tag)
		b.WriteString("段\n")
		b.WriteString("这是" + tag + "段的合成正文，用来验证导入链路，与真实作品无关。\n\n")
	}
	return b.String()
}

// itoa 是测试用的小整数转字符串。
func itoa(n int) string { return strconv.Itoa(n) }

// collectionText 合成两书合集：第一本 first 章，第二本 second 章（章号重新从 1 开始）。
func collectionText(first, second int, secondTitle string) string {
	var b strings.Builder
	b.WriteString("书名：合成合集\n作者：某合集\n\n")
	b.WriteString(chaptersBlock(first, "甲"))
	if secondTitle != "" {
		b.WriteString("书名：" + secondTitle + "\n作者：某续\n\n")
	}
	b.WriteString(chaptersBlock(second, "乙"))
	return b.String()
}

// chapterIDs 取一本书的章节 id（按目录顺序）。
func chapterIDs(t *testing.T, st *store.Store, bookID int64) []int64 {
	t.Helper()
	page, err := st.ListChapters(context.Background(), store.ChapterQuery{BookID: bookID, From: 1, Limit: 500})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	ids := make([]int64, 0, len(page.Items))
	for _, c := range page.Items {
		ids = append(ids, c.ID)
	}
	return ids
}

// TestRunSplitsCollection 覆盖两段合集：各成一本、SplitOf 指向原文件、章数正确。
func TestRunSplitsCollection(t *testing.T) {
	dir := t.TempDir()
	// 文件名优先作为书名（parser 的既定规则），所以文件名与第一段书名保持一致。
	writeTxt(t, dir, "合成合集.txt", collectionText(20, 12, "合成续卷"))
	ctx := context.Background()
	st := newStore(t)
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 2 || res.Failed != 0 || res.Skipped != 0 {
		t.Fatalf("导入 %d 失败 %d 跳过 %d，期望 2/0/0（errors=%v）", len(res.Imported), res.Failed, res.Skipped, res.Errors)
	}
	books, err := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	assertSplitBooks(t, ctx, st, books)
}

// assertSplitBooks 校验拆分后的两本书：书名、章数、来源名。
func assertSplitBooks(t *testing.T, ctx context.Context, st *store.Store, books store.Page[store.BookSummary]) {
	t.Helper()
	if books.Total != 2 {
		t.Fatalf("书架 = %d 本，期望 2", books.Total)
	}
	want := map[string]int{"合成合集": 20, "合成续卷": 12}
	for _, summary := range books.Items {
		assertSplitBook(t, ctx, st, summary, want)
	}
}

// assertSplitBook 校验一本书的章数与来源名。
func assertSplitBook(t *testing.T, ctx context.Context, st *store.Store, summary store.BookSummary, want map[string]int) {
	t.Helper()
	n, ok := want[summary.Title]
	if !ok || summary.ChapterCount != n {
		t.Errorf("%q 章数 = %d，期望 %v", summary.Title, summary.ChapterCount, want)
	}
	detail, err := st.GetBook(ctx, summary.ID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if !strings.HasSuffix(detail.SourceName, "合集.txt") {
		t.Errorf("%q 来源名 = %q，期望沿用原文件名", summary.Title, detail.SourceName)
	}
}

// TestRunSplitReimportHitsSameSegment 覆盖重导同一合集：两段都按来源路径命中，不再新建书。
func TestRunSplitReimportHitsSameSegment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "合成合集.txt")
	writeTxt(t, dir, "合成合集.txt", collectionText(20, 12, "合成续卷"))
	ctx := context.Background()
	st := newStore(t)
	if _, err := Run(ctx, st, opts(dir), nil); err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	// 内容变了（第二段多一章），但两段的来源路径仍应命中。
	if err := os.WriteFile(path, []byte(collectionText(20, 13, "合成续卷")), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, st, opts(dir), nil)
	if err != nil {
		t.Fatalf("重导: %v", err)
	}
	if len(res.Imported) != 0 || res.Skipped != 2 {
		t.Fatalf("重导导入 %d 跳过 %d，期望 0/2（errors=%v）", len(res.Imported), res.Skipped, res.Errors)
	}
	books, _ := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if books.Total != 2 {
		t.Fatalf("重导后书架 = %d 本，期望仍是 2", books.Total)
	}
}

// TestRunShortSegmentNotSplit 覆盖「疑似合集但每段不足 3 章」：不拆，按单本导入。
func TestRunShortSegmentNotSplit(t *testing.T) {
	dir := t.TempDir()
	writeTxt(t, dir, "短段.txt", collectionText(12, 2, "合成短段"))
	st := newStore(t)
	res, err := Run(context.Background(), st, opts(dir), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("导入 %d 本，期望 1（不足 3 章的段不拆）", len(res.Imported))
	}
	detail, err := st.GetBook(context.Background(), res.Imported[0])
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.ChapterCount != 14 {
		t.Errorf("章数 = %d，期望 14（不拆则保留全部章）", detail.ChapterCount)
	}
	if detail.Title != "短段" {
		t.Errorf("书名 = %q，期望沿用原书名（不拆不换名）", detail.Title)
	}
}

// TestRunUpdateAppendsTail 覆盖追更：10 章 → 15 章只追加尾部，旧章 id 与阅读进度不动。
func TestRunUpdateAppendsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "追更.txt")
	writeTxt(t, dir, "追更.txt", "书名：合成追更\n作者：某追\n\n"+chaptersBlock(10, "旧"))
	ctx := context.Background()
	st := newStore(t)
	bookID, oldIDs := importTenChapters(t, ctx, st, dir)
	if err := os.WriteFile(path, []byte("书名：合成追更\n作者：某追\n\n"+chaptersBlock(15, "旧")), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, st, updateOpts(dir), nil)
	if err != nil {
		t.Fatalf("追更: %v", err)
	}
	if len(res.Imported) != 1 || res.Failed != 0 {
		t.Fatalf("追更导入 %d 失败 %d，期望 1/0（errors=%v）", len(res.Imported), res.Failed, res.Errors)
	}
	assertAppended(t, ctx, st, bookID, oldIDs)
}

// importTenChapters 首次导入 10 章并写一条阅读进度，返回书 id 与章节 id。
func importTenChapters(t *testing.T, ctx context.Context, st *store.Store, dir string) (int64, []int64) {
	t.Helper()
	first, err := Run(ctx, st, opts(dir), nil)
	if err != nil || len(first.Imported) != 1 {
		t.Fatalf("首次导入: err=%v imported=%v", err, first.Imported)
	}
	bookID := first.Imported[0]
	oldIDs := chapterIDs(t, st, bookID)
	if len(oldIDs) != 10 {
		t.Fatalf("首次章数 = %d，期望 10", len(oldIDs))
	}
	if err := st.SaveReadingProgress(ctx, store.ReadingProgress{
		BookID: bookID, ChapterID: oldIDs[4], ChapterIdx: 4, Percent: 0.4,
	}); err != nil {
		t.Fatalf("SaveReadingProgress: %v", err)
	}
	return bookID, oldIDs
}

// updateOpts 返回追更模式的导入参数。
func updateOpts(dir string) Options {
	opt := opts(dir)
	opt.Update = true
	return opt
}

// assertAppended 断言追更结果：章数变 15、旧章 id 不变、阅读进度仍可读。
func assertAppended(t *testing.T, ctx context.Context, st *store.Store, bookID int64, oldIDs []int64) {
	t.Helper()
	newIDs := chapterIDs(t, st, bookID)
	if len(newIDs) != 15 {
		t.Fatalf("追更后章数 = %d，期望 15", len(newIDs))
	}
	for i, id := range oldIDs {
		if newIDs[i] != id {
			t.Fatalf("第 %d 章 id = %d，期望原 id %d 不变", i+1, newIDs[i], id)
		}
	}
	items, err := st.ListReadingProgress(ctx, 5)
	if err != nil || len(items) != 1 || items[0].ChapterID != oldIDs[4] {
		t.Fatalf("阅读进度 = %+v, err=%v，期望仍指向原第 5 章", items, err)
	}
}

// TestRunUpdateRejectsReordered 覆盖追更拒绝：标题序列对不上 → 失败并记 ErrNotAppendable。
func TestRunUpdateRejectsReordered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "追更.txt")
	writeTxt(t, dir, "追更.txt", "书名：合成追更\n作者：某追\n\n"+chaptersBlock(10, "旧"))
	ctx := context.Background()
	st := newStore(t)
	first, err := Run(ctx, st, opts(dir), nil)
	if err != nil || len(first.Imported) != 1 {
		t.Fatalf("首次导入: err=%v imported=%v", err, first.Imported)
	}
	// 第 3 章标题被改写，前缀对不上。
	broken := strings.Replace("书名：合成追更\n作者：某追\n\n"+chaptersBlock(15, "旧"), "第3章 旧段", "第3章 改名段", 1)
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	upOpt := opts(dir)
	upOpt.Update = true
	res, err := Run(ctx, st, upOpt, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 0 || res.Failed != 1 {
		t.Fatalf("导入 %d 失败 %d，期望 0/1（倒序/改名必须拒绝）", len(res.Imported), res.Failed)
	}
	if !strings.Contains(res.Errors[0].Reason, "not an extension") {
		t.Errorf("失败原因 = %q，期望说明「不是已有章节的追加」", res.Errors[0].Reason)
	}
	if got := chapterIDs(t, st, first.Imported[0]); len(got) != 10 {
		t.Errorf("拒绝后章数 = %d，期望仍是 10（绝不覆盖）", len(got))
	}
}
