package store

import (
	"context"
	"errors"
	"testing"
)

// 断言辅助：把分支收进这些小函数，用例本体就能保持「一行一条断言」。
// 这也让复杂度门禁不再把平铺的测试断言误当成业务分支（新增测试同样要 ≤ 10）。

// assertEqual 断言两个可比较值相等。
func assertEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v，期望 %v", name, got, want)
	}
}

// requireLen 断言长度并原样返回切片，用例可以直接取下标。
func requireLen[T any](t *testing.T, name string, items []T, want int) []T {
	t.Helper()
	if len(items) != want {
		t.Fatalf("%s = %d，期望 %d（%+v）", name, len(items), want, items)
	}
	return items
}

// execOrFail 执行一条测试用 SQL。
func execOrFail(t *testing.T, st *Store, sqlText string, args ...any) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(), sqlText, args...); err != nil {
		t.Fatalf("执行 SQL 失败: %v\n%s", err, sqlText)
	}
}

// clearAuthorOf 把一本书的作者显式清空（author_id=0 + 删关联行），
// 这是 /api/health/report 里 missingAuthor 认的那种「缺作者」。
func clearAuthorOf(t *testing.T, st *Store, bookID int64) {
	t.Helper()
	execOrFail(t, st, "DELETE FROM books_authors WHERE book_id = ?", bookID)
	execOrFail(t, st, "UPDATE books SET author_id = 0 WHERE id = ?", bookID)
}

// seedIssueFixture 造一个「四类问题各一条」的库，供健康明细用例共用。
//
// 四本书分别是：正常书 / 缺作者 / 缺简介 / 章节不足，另外把正常书的第一章改成超长。
func seedIssueFixture(t *testing.T, st *Store) {
	t.Helper()
	books := []struct{ title, author, path, sha string }{
		{"问题样本甲", "作者甲", "/books/issue-a.txt", repeat64("a")},
		{"问题样本乙", "作者乙", "/books/issue-b.txt", repeat64("b")},
		{"问题样本丙", "作者丙", "/books/issue-c.txt", repeat64("c")},
		{"问题样本丁", "作者丁", "/books/issue-d.txt", repeat64("d")},
	}
	ids := make(map[string]int64, len(books))
	for _, item := range books {
		res, err := st.SaveBook(context.Background(),
			sampleBook(item.title, item.author, item.path, item.sha), SaveOptions{})
		requireNoErr(t, err, item.title)
		ids[item.title] = res.BookID
	}
	// 缺作者：把作者显式清空（导入时空作者会落「佚名」，那不是「缺作者」）。
	clearAuthorOf(t, st, ids["问题样本乙"])
	// 缺简介：清空简介；章节不足：章节数改成 1。
	execOrFail(t, st, "UPDATE books SET intro = '' WHERE id = ?", ids["问题样本丙"])
	execOrFail(t, st, "UPDATE books SET chapter_count = 1 WHERE id = ?", ids["问题样本丁"])
	// 超长章节：甲书的第一章改成 4 万字（阈值 3 万）。
	execOrFail(t, st, "UPDATE chapters SET word_count = 40000 WHERE book_id = ? AND idx = 0", ids["问题样本甲"])
}

// requireIssues 取一类问题的明细页并要求成功。
func requireIssues(t *testing.T, st *Store, kind string, page, size int) HealthIssues {
	t.Helper()
	got, err := st.HealthIssues(context.Background(), kind, page, size)
	requireNoErr(t, err, kind)
	return got
}

// TestHealthIssuesMatchesReport 是「口径一致」的防退化测试：
// 每个 kind 的 total 必须等于 /api/health/report 里对应的那个数字。
func TestHealthIssuesMatchesReport(t *testing.T) {
	st := newTestStore(t)
	seedIssueFixture(t, st)
	report := st.HealthReport(context.Background())
	want := map[string]*int{
		HealthIssueMissingAuthor: report.MissingAuthor,
		HealthIssueMissingIntro:  report.MissingIntro,
		HealthIssueHugeChapter:   report.HugeChapters,
		HealthIssueThinBook:      report.ThinBooks,
	}
	for kind, expected := range want {
		assertIntPtr(t, kind+" 报告值", expected, 1)
		got := requireIssues(t, st, kind, 1, 50)
		assertEqual(t, kind+" 明细 total", got.Total, *expected)
		assertEqual(t, kind+" 明细条数", len(got.Items), 1)
	}
}

// TestHealthIssuesBookLevelFields 覆盖书级明细的字段（书名 / 作者 / 章节数 / 字数）。
func TestHealthIssuesBookLevelFields(t *testing.T) {
	st := newTestStore(t)
	seedIssueFixture(t, st)
	missing := requireIssues(t, st, HealthIssueMissingAuthor, 1, 10)
	item := requireLen(t, "缺作者条数", missing.Items, 1)[0]

	assertEqual(t, "书名", item.Title, "问题样本乙")
	assertEqual(t, "作者", item.Author, "")
	assertEqual(t, "章节数", item.ChapterCount, 3)
	assertEqual(t, "书级条目不带 chapterId", item.ChapterID, int64(0))
	assertEqual(t, "书级条目不带 chapterIdx", item.ChapterIdx, (*int)(nil))
	assertEqual(t, "kind", missing.Kind, HealthIssueMissingAuthor)
	assertEqual(t, "page", missing.Page, 1)
	assertEqual(t, "size", missing.Size, 10)
}

// TestHealthIssuesChapterLevelFields 覆盖章节级明细的字段（书 + 章 + 本章字数）。
func TestHealthIssuesChapterLevelFields(t *testing.T) {
	st := newTestStore(t)
	seedIssueFixture(t, st)
	item := requireLen(t, "超长章节条数", requireIssues(t, st, HealthIssueHugeChapter, 1, 10).Items, 1)[0]

	assertEqual(t, "本章字数", item.WordCount, 40000)
	assertEqual(t, "章节有 id", item.ChapterID > 0, true)
	assertEqual(t, "章节标题非空", item.ChapterTitle != "", true)
	assertEqual(t, "书名非空", item.Title != "", true)
	// 0 号章的下标必须是真的 0，不能被 omitempty 吃掉（前端要显示「第 1 章」）。
	if item.ChapterIdx == nil {
		t.Fatal("chapterIdx 为 nil，期望指向 0 号章")
	}
	assertEqual(t, "chapterIdx", *item.ChapterIdx, 0)
}

// TestHealthIssuesPagination 覆盖分页：两页不重复、越界页返回空列表。
func TestHealthIssuesPagination(t *testing.T) {
	st := newTestStore(t)
	for i, title := range []string{"分页一", "分页二", "分页三"} {
		res, err := st.SaveBook(context.Background(),
			sampleBook(title, "分页作者", "/books/page-"+title+".txt", repeat64(string(rune('a'+i)))), SaveOptions{})
		requireNoErr(t, err, title)
		clearAuthorOf(t, st, res.BookID)
	}
	first := requireIssues(t, st, HealthIssueMissingAuthor, 1, 2)
	assertEqual(t, "第 1 页 total", first.Total, 3)
	firstItems := requireLen(t, "第 1 页条数", first.Items, 2)

	second := requireIssues(t, st, HealthIssueMissingAuthor, 2, 2)
	assertEqual(t, "第 2 页 total", second.Total, 3)
	secondItems := requireLen(t, "第 2 页条数", second.Items, 1)
	assertEqual(t, "两页不重复", firstItems[0].BookID != secondItems[0].BookID, true)

	// 越界页返回空列表（不是错误），前端据此显示「没有更多」。
	third := requireIssues(t, st, HealthIssueMissingAuthor, 3, 2)
	assertEqual(t, "越界页 total", third.Total, 3)
	assertEqual(t, "越界页条数", len(third.Items), 0)
}

// TestHealthIssuesRejectsBadInput 覆盖 kind / page / size 的入参校验。
func TestHealthIssuesRejectsBadInput(t *testing.T) {
	st := newTestStore(t)
	cases := []struct {
		name string
		kind string
		page int
		size int
	}{
		{"未知 kind", "bogus", 1, 20},
		{"空 kind", "", 1, 20},
		{"page=0", HealthIssueMissingAuthor, 0, 20},
		{"size=0", HealthIssueMissingAuthor, 1, 0},
		{"size 为负", HealthIssueMissingAuthor, 1, -5},
	}
	for _, c := range cases {
		_, err := st.HealthIssues(context.Background(), c.kind, c.page, c.size)
		assertEqual(t, c.name+" 应被拒绝", errors.Is(err, ErrInvalidArgument), true)
	}
}

// TestHealthIssuesEmpty 覆盖空库：items 是 [] 而不是 null，total 为 0。
func TestHealthIssuesEmpty(t *testing.T) {
	st := newTestStore(t)
	got := requireIssues(t, st, HealthIssueMissingIntro, 1, 20)
	assertEqual(t, "空库 items 非 nil", got.Items != nil, true)
	assertEqual(t, "空库 total", got.Total, 0)
	assertEqual(t, "空库条数", len(got.Items), 0)
}
