package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 断言辅助：把分支收进这些小函数，用例本体保持「一行一条断言」，
// 复杂度门禁就不会把平铺的测试断言当成业务分支（新测试同样要 ≤ 10）。

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

// execSQL 执行一条测试用 SQL。
func (e *testEnv) execSQL(sqlText string, args ...any) {
	e.t.Helper()
	if _, err := e.st.DB().Exec(sqlText, args...); err != nil {
		e.t.Fatalf("执行 SQL 失败: %v\n%s", err, sqlText)
	}
}

// clearAuthor 把一本书的作者显式清空（与 /api/health/report 的 missingAuthor 同口径）。
func (e *testEnv) clearAuthor(bookID int64) {
	e.t.Helper()
	e.execSQL("DELETE FROM books_authors WHERE book_id = ?", bookID)
	e.execSQL("UPDATE books SET author_id = 0 WHERE id = ?", bookID)
}

// reportCounts 是健康报告里四项问题计数的快照（测试用）。
type reportCounts struct {
	missingAuthor *int
	missingIntro  *int
	thinBooks     *int
	hugeChapters  *int
}

// healthCounts 取 /api/health/report 的四项计数。
func (e *testEnv) healthCounts() reportCounts {
	e.t.Helper()
	body := decodeBody[healthReportBody](e.t, e.get("/api/health/report"))
	return reportCounts{
		missingAuthor: body.MissingAuthor,
		missingIntro:  body.MissingIntro,
		thinBooks:     body.ThinBooks,
		hugeChapters:  body.HugeChapters,
	}
}

// healthIssuesBody 是健康明细响应（测试用）。
type healthIssuesBody struct {
	Kind  string             `json:"kind"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
	Items []healthIssueEntry `json:"items"`
}

// healthIssueEntry 是健康明细里的一条（测试用）。
type healthIssueEntry struct {
	BookID       int64  `json:"bookId"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	ChapterCount int    `json:"chapterCount"`
	WordCount    int    `json:"wordCount"`
	ChapterID    int64  `json:"chapterId"`
	ChapterIdx   *int   `json:"chapterIdx"`
	ChapterTitle string `json:"chapterTitle"`
}

// issues 取一类问题的明细并要求 200。
func (e *testEnv) issues(query string) healthIssuesBody {
	e.t.Helper()
	rr := e.get("/api/health/issues?" + query)
	if rr.Code != http.StatusOK {
		e.t.Fatalf("GET health/issues?%s = %d（body=%s）", query, rr.Code, rr.Body.String())
	}
	return decodeBody[healthIssuesBody](e.t, rr)
}

// TestHealthIssuesEmpty 校验空库返回 [] 而非 null，且 total 为 0。
func TestHealthIssuesEmpty(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/health/issues?kind=missing_intro")
	assertEqual(t, "状态码", rr.Code, http.StatusOK)
	var raw struct {
		Items json.RawMessage `json:"items"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("解析: %v", err)
	}
	assertEqual(t, "空库 items", string(raw.Items), "[]")
	assertEqual(t, "空库 total", raw.Total, 0)
}

// TestHealthIssuesRejectsBadInput 覆盖 kind / page / size 的 400（不回落默认值）。
func TestHealthIssuesRejectsBadInput(t *testing.T) {
	e := newEnv(t)
	cases := []string{
		"/api/health/issues?kind=bogus",
		"/api/health/issues",
		"/api/health/issues?kind=",
		"/api/health/issues?kind=missing_author&page=0",
		"/api/health/issues?kind=missing_author&size=201",
		"/api/health/issues?kind=missing_author&size=abc",
	}
	for _, target := range cases {
		requireError(t, e.get(target), http.StatusBadRequest, codeInvalidParam)
	}
}

// TestHealthIssuesMissingAuthor 覆盖书级明细字段与「与报告同口径」。
func TestHealthIssuesMissingAuthor(t *testing.T) {
	e := newEnv(t)
	noAuthor := e.seedBook("缺作者之书", "会被清空的作者")
	e.clearAuthor(noAuthor)
	e.seedBook("正常之书", "正常作者")

	report := e.healthCounts()
	assertEqual(t, "报告缺作者数", *report.missingAuthor, 1)
	body := e.issues("kind=missing_author")
	assertEqual(t, "明细 total 与报告一致", body.Total, *report.missingAuthor)
	item := requireLen(t, "缺作者条数", body.Items, 1)[0]
	assertEqual(t, "书名", item.Title, "缺作者之书")
	assertEqual(t, "作者为空", item.Author, "")
	assertEqual(t, "章节数", item.ChapterCount, 3)
	assertEqual(t, "字数非零", item.WordCount > 0, true)
	assertEqual(t, "kind", body.Kind, "missing_author")
	assertEqual(t, "page 默认 1", body.Page, 1)
	assertEqual(t, "size 默认 20", body.Size, 20)
}

// TestHealthIssuesMissingIntro 覆盖缺简介的判定与字段。
func TestHealthIssuesMissingIntro(t *testing.T) {
	e := newEnv(t)
	noIntro := e.seedBook("缺简介之书", "作者乙")
	e.execSQL("UPDATE books SET intro = '' WHERE id = ?", noIntro)
	e.seedBook("有简介之书", "作者丙")

	report := e.healthCounts()
	assertEqual(t, "报告缺简介数", *report.missingIntro, 1)
	body := e.issues("kind=missing_intro")
	assertEqual(t, "明细 total 与报告一致", body.Total, *report.missingIntro)
	item := requireLen(t, "缺简介条数", body.Items, 1)[0]
	assertEqual(t, "书名", item.Title, "缺简介之书")
	assertEqual(t, "作者", item.Author, "作者乙")
}

// TestHealthIssuesThinBook 覆盖「章节完整」下钻（章节数 ≤ 1）。
func TestHealthIssuesThinBook(t *testing.T) {
	e := newEnv(t)
	thin := e.seedBook("章节极少的书", "作者丁")
	e.execSQL("UPDATE books SET chapter_count = 1 WHERE id = ?", thin)

	report := e.healthCounts()
	assertEqual(t, "报告章节不足数", *report.thinBooks, 1)
	body := e.issues("kind=thin_book")
	assertEqual(t, "明细 total 与报告一致", body.Total, *report.thinBooks)
	item := requireLen(t, "章节不足条数", body.Items, 1)[0]
	assertEqual(t, "章节数", item.ChapterCount, 1)
}

// TestHealthIssuesHugeChapter 覆盖章节级明细：书 + 章 + 本章字数。
func TestHealthIssuesHugeChapter(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("超长章节之书", "作者戊")
	e.execSQL("UPDATE chapters SET word_count = 45000 WHERE book_id = ? AND idx = 0", bookID)

	report := e.healthCounts()
	assertEqual(t, "报告超长章节数", *report.hugeChapters, 1)
	body := e.issues("kind=huge_chapter")
	assertEqual(t, "明细 total 与报告一致", body.Total, *report.hugeChapters)
	item := requireLen(t, "超长章节条数", body.Items, 1)[0]
	assertEqual(t, "书 id", item.BookID, bookID)
	assertEqual(t, "书名", item.Title, "超长章节之书")
	assertEqual(t, "本章字数", item.WordCount, 45000)
	assertEqual(t, "章节 id 有效", item.ChapterID > 0, true)
	assertEqual(t, "章节标题非空", item.ChapterTitle != "", true)
	if item.ChapterIdx == nil {
		t.Fatal("chapterIdx 为 nil，期望指向 0 号章")
	}
	assertEqual(t, "chapterIdx", *item.ChapterIdx, 0)
}

// TestHealthIssuesPagination 覆盖分页：两页不重复、越界页 200 + 空列表。
func TestHealthIssuesPagination(t *testing.T) {
	e := newEnv(t)
	for _, title := range []string{"分页书一", "分页书二", "分页书三"} {
		e.clearAuthor(e.seedBook(title, "会被清空的作者"))
	}
	first := e.issues("kind=missing_author&page=1&size=2")
	assertEqual(t, "第 1 页 total", first.Total, 3)
	assertEqual(t, "第 1 页 size", first.Size, 2)
	firstItems := requireLen(t, "第 1 页条数", first.Items, 2)

	second := e.issues("kind=missing_author&page=2&size=2")
	assertEqual(t, "第 2 页 total", second.Total, 3)
	secondItems := requireLen(t, "第 2 页条数", second.Items, 1)
	assertEqual(t, "两页不重复", firstItems[0].BookID != secondItems[0].BookID, true)

	// 越界页：200 + 空列表（不是 404），前端据此显示「没有更多」。
	third := e.issues("kind=missing_author&page=9&size=2")
	assertEqual(t, "越界页 total", third.Total, 3)
	assertEqual(t, "越界页条数", len(third.Items), 0)
}
