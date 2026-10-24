package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// putJSON 发一个 PUT JSON 请求。
func (e *testEnv) putJSON(target string, payload any) *httptest.ResponseRecorder {
	e.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		e.t.Fatalf("序列化请求体: %v", err)
	}
	return e.do(http.MethodPut, target, bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
	})
}

// bookChapters 取某本书的章节（测试辅助）。
func (e *testEnv) bookChapters(bookID int64) chapterListResponse {
	e.t.Helper()
	return decodeBody[chapterListResponse](e.t,
		e.get("/api/books/"+strconv.FormatInt(bookID, 10)+"/chapters?limit=10"))
}

// progressItems 读最近在读列表。
func (e *testEnv) progressItems() progressListResponse {
	e.t.Helper()
	return decodeBody[progressListResponse](e.t, e.get("/api/reading/progress"))
}

// TestProgressListEmpty 校验空表返回 [] 而非 null。
func TestProgressListEmpty(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/reading/progress")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET progress = %d", rr.Code)
	}
	var raw struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if string(raw.Items) != "[]" {
		t.Errorf("空表 items = %s，期望 []", raw.Items)
	}
}

// TestProgressPutAndUpsert 覆盖 PUT 新建、联表字段与 upsert。
func TestProgressPutAndUpsert(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("进度之书", "进度作者")
	chapters := e.bookChapters(bookID)
	if len(chapters.Items) < 2 {
		t.Fatalf("种子书章节数 = %d，期望 ≥ 2", len(chapters.Items))
	}
	ch1, ch2 := chapters.Items[0].ID, chapters.Items[1].ID

	if rr := e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID, "chapterId": ch1, "percent": 0.25,
	}); rr.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d，期望 204（body=%s）", rr.Code, rr.Body.String())
	}
	assertSingleProgress(t, e.progressItems(), bookID, ch1, 0.25)

	// upsert：同一本书换章节不新增行。
	if rr := e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID, "chapterId": ch2, "percent": 0.8,
	}); rr.Code != http.StatusNoContent {
		t.Fatalf("二次 PUT = %d", rr.Code)
	}
	assertSingleProgress(t, e.progressItems(), bookID, ch2, 0.8)
}

// assertSingleProgress 校验列表只有一条且字段正确。
func assertSingleProgress(t *testing.T, got progressListResponse, bookID, chapterID int64, percent float64) {
	t.Helper()
	if len(got.Items) != 1 {
		t.Fatalf("列表条数 = %d，期望 1（%+v）", len(got.Items), got.Items)
	}
	p := got.Items[0]
	if p.BookID != bookID || p.ChapterID != chapterID || p.Percent != percent {
		t.Errorf("进度 = %+v", p)
	}
	if p.BookTitle == "" || p.ChapterTitle == "" {
		t.Errorf("联表字段缺失: book=%q chapter=%q", p.BookTitle, p.ChapterTitle)
	}
}

// TestProgressLimitBounds 覆盖 limit 参数边界。
func TestProgressLimitBounds(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.get("/api/reading/progress?limit=0"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/reading/progress?limit=999"), http.StatusBadRequest, codeInvalidParam)
	if rr := e.get("/api/reading/progress?limit=5"); rr.Code != http.StatusOK {
		t.Errorf("limit=5 → %d", rr.Code)
	}
}

// TestProgressPutRejectsMalformedBody 覆盖请求体形状错误。
func TestProgressPutRejectsMalformedBody(t *testing.T) {
	e := newEnv(t)
	cases := []struct{ name, body string }{
		{"空 body", ""},
		{"非 JSON", "not json"},
		{"多对象", `{"bookId":1,"chapterId":1,"percent":0.1}{"x":1}`},
	}
	for _, c := range cases {
		rr := e.do(http.MethodPut, "/api/reading/progress",
			bytes.NewBufferString(c.body), map[string]string{"Content-Type": "application/json"})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: 状态码 = %d，期望 400（body=%s）", c.name, rr.Code, rr.Body.String())
			continue
		}
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
}

// TestProgressPutRejectsBadFields 覆盖字段级校验。
func TestProgressPutRejectsBadFields(t *testing.T) {
	e := newEnv(t)
	e.seedBook("校验之书", "校验作者")
	cases := []struct{ name, body string }{
		{"缺 bookId", `{"chapterId":1,"percent":0.1}`},
		{"缺 chapterId", `{"bookId":1,"percent":0.1}`},
		{"bookId 非正", `{"bookId":0,"chapterId":1,"percent":0.1}`},
		{"percent 超上界", `{"bookId":1,"chapterId":1,"percent":1.1}`},
		{"percent 为负", `{"bookId":1,"chapterId":1,"percent":-0.1}`},
	}
	for _, c := range cases {
		rr := e.do(http.MethodPut, "/api/reading/progress",
			bytes.NewBufferString(c.body), map[string]string{"Content-Type": "application/json"})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: 状态码 = %d，期望 400（body=%s）", c.name, rr.Code, rr.Body.String())
			continue
		}
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
}

// TestProgressPutUnknownChapter 覆盖「章节不属于本书」→ 404。
func TestProgressPutUnknownChapter(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("404 之书", "404 作者")
	chapters := e.bookChapters(bookID)
	rr := e.putJSON("/api/reading/progress", map[string]any{
		"bookId": bookID + 900, "chapterId": chapters.Items[0].ID, "percent": 0.1,
	})
	requireError(t, rr, http.StatusNotFound, codeNotFound)
}

// TestHealthReportEndpoint 覆盖 GET /api/health/report 的整体形状。
func TestHealthReportEndpoint(t *testing.T) {
	e := newEnv(t)
	e.seedBook("健康之书甲", "作者甲")
	e.seedBook("健康之书乙", "")

	rr := e.get("/api/health/report")
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/health/report = %d（body=%s）", rr.Code, rr.Body.String())
	}
	body := decodeBody[healthReportBody](t, rr)
	if body.TotalBooks == nil || *body.TotalBooks != 2 {
		t.Errorf("totalBooks = %v，期望 2", body.TotalBooks)
	}
	if body.Warnings == nil {
		t.Error("warnings 应为 [] 而非 null")
	}
	if len(body.ChapterBuckets) == 0 || len(body.ChapterLengthBuckets) == 0 {
		t.Errorf("分桶缺失: %d / %d", len(body.ChapterBuckets), len(body.ChapterLengthBuckets))
	}
}

// TestHealthReportBucketSums 覆盖分桶合计与桶位归属。
func TestHealthReportBucketSums(t *testing.T) {
	e := newEnv(t)
	e.seedBook("分桶之书甲", "作者甲")
	e.seedBook("分桶之书乙", "作者乙")
	body := decodeBody[healthReportBody](t, e.get("/api/health/report"))

	sum := 0
	for _, b := range body.ChapterBuckets {
		sum += b.Count
	}
	if sum != 2 {
		t.Errorf("章节分桶合计 = %d，期望 2", sum)
	}
	// 每本种子书 3 章 → 都落在「2-9」桶。
	for _, b := range body.ChapterBuckets {
		if b.Label == "2-9" && b.Count != 2 {
			t.Errorf("2-9 桶 = %d，期望 2", b.Count)
		}
	}
}

// healthReportBody 是健康报告响应（测试用）。
type healthReportBody struct {
	TotalBooks           *int     `json:"totalBooks"`
	ThinBooks            *int     `json:"thinBooks"`
	MissingAuthor        *int     `json:"missingAuthor"`
	MissingIntro         *int     `json:"missingIntro"`
	HugeChapters         *int     `json:"hugeChapters"`
	ChapterBuckets       []bucket `json:"chapterBuckets"`
	ChapterLengthBuckets []bucket `json:"chapterLengthBuckets"`
	Warnings             []string `json:"warnings"`
}

// bucket 是分桶响应结构（测试用）。
type bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// TestImportTrendEndpoint 覆盖趋势端点形状与补 0。
func TestImportTrendEndpoint(t *testing.T) {
	e := newEnv(t)
	e.seedBook("趋势之书", "趋势作者")

	rr := e.get("/api/stats/import-trend?days=3")
	if rr.Code != http.StatusOK {
		t.Fatalf("import-trend = %d（body=%s）", rr.Code, rr.Body.String())
	}
	var points []trendPoint
	if err := json.Unmarshal(rr.Body.Bytes(), &points); err != nil {
		t.Fatalf("解析趋势: %v（body=%s）", err, rr.Body.String())
	}
	if len(points) != 3 {
		t.Fatalf("天数 = %d，期望 3", len(points))
	}
	if points[0].Date == "" || points[2].Date == "" {
		t.Error("日期不能为空")
	}
	if points[2].Books != 1 || points[2].Chapters != 3 {
		t.Errorf("今天 = %+v，期望 books=1 chapters=3", points[2])
	}
}

// TestImportTrendZeroFilled 覆盖无数据日期补 0。
func TestImportTrendZeroFilled(t *testing.T) {
	e := newEnv(t)
	e.seedBook("补零之书", "补零作者")
	var points []trendPoint
	if err := json.Unmarshal(e.get("/api/stats/import-trend?days=3").Body.Bytes(), &points); err != nil {
		t.Fatalf("解析: %v", err)
	}
	for _, p := range points[:len(points)-1] {
		if p.Books != 0 || p.Chapters != 0 {
			t.Errorf("%s 应补 0，实际 %+v", p.Date, p)
		}
	}
}

// trendPoint 是趋势响应元素（测试用）。
type trendPoint struct {
	Date     string `json:"date"`
	Books    int    `json:"books"`
	Chapters int    `json:"chapters"`
}

// TestImportTrendDefaultsAndBounds 覆盖默认天数与非法入参。
func TestImportTrendDefaultsAndBounds(t *testing.T) {
	e := newEnv(t)
	var def []trendPoint
	if err := json.Unmarshal(e.get("/api/stats/import-trend").Body.Bytes(), &def); err != nil {
		t.Fatalf("默认趋势解析: %v", err)
	}
	if len(def) != 14 {
		t.Errorf("默认天数 = %d，期望 14", len(def))
	}
	requireError(t, e.get("/api/stats/import-trend?days=0"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/stats/import-trend?days=abc"), http.StatusBadRequest, codeInvalidParam)
}

// TestAuthorsSortWords 覆盖 sort=words 把字数最多的排在首位。
func TestAuthorsSortWords(t *testing.T) {
	e := newEnv(t)
	e.seedBook("少字书", "少字作者")
	more := e.seedBook("多字书", "多字作者")
	if _, err := e.st.DB().Exec("UPDATE books SET word_count = 999999 WHERE id = ?", more); err != nil {
		t.Fatalf("更新字数: %v", err)
	}

	rr := e.get("/api/authors?sort=words&size=10")
	if rr.Code != http.StatusOK {
		t.Fatalf("authors?sort=words = %d（body=%s）", rr.Code, rr.Body.String())
	}
	body := decodeBody[authorItems](t, rr)
	if len(body.Items) == 0 || body.Items[0].Name != "多字作者" {
		t.Errorf("字数首位 = %+v，期望 多字作者", body.Items)
	}
}

// authorItems 是作者列表响应（测试用）。
type authorItems struct {
	Items []struct {
		Name      string `json:"name"`
		WordCount int    `json:"wordCount"`
	} `json:"items"`
}

// TestAuthorsSortBackwardCompatible 覆盖默认与既有排序仍可用。
func TestAuthorsSortBackwardCompatible(t *testing.T) {
	e := newEnv(t)
	e.seedBook("兼容之书", "兼容作者")
	for _, sortKey := range []string{"", "books", "name"} {
		if rr := e.get("/api/authors?sort=" + sortKey); rr.Code != http.StatusOK {
			t.Errorf("sort=%q → %d", sortKey, rr.Code)
		}
	}
	requireError(t, e.get("/api/authors?sort=bogus"), http.StatusBadRequest, codeInvalidParam)
}

// TestStatsHasComposition 覆盖 /api/stats 的体积构成扩展。
func TestStatsHasComposition(t *testing.T) {
	e := newEnv(t)
	e.seedBook("体积之书", "体积作者")

	rr := e.get("/api/stats")
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/stats = %d", rr.Code)
	}
	var s statsComposition
	if err := json.Unmarshal(rr.Body.Bytes(), &s); err != nil {
		t.Fatalf("解析 stats: %v", err)
	}
	if s.DBBytes <= 0 {
		t.Errorf("dbBytes = %d", s.DBBytes)
	}
	if sum := s.TextBytes + s.IndexBytes + s.OtherBytes; sum != s.DBBytes {
		t.Errorf("体积构成之和 %d != dbBytes %d", sum, s.DBBytes)
	}
	if s.OtherBytes < 0 {
		t.Errorf("otherBytes = %d 不应为负", s.OtherBytes)
	}
}

// statsComposition 是 /api/stats 的体积构成字段（测试用）。
type statsComposition struct {
	DBBytes    int64 `json:"dbBytes"`
	TextBytes  int64 `json:"textBytes"`
	IndexBytes int64 `json:"indexBytes"`
	OtherBytes int64 `json:"otherBytes"`
}

// TestNewRoutesMethodRules 覆盖新端点的方法错误与未知路径。
func TestNewRoutesMethodRules(t *testing.T) {
	e := newEnv(t)
	rr := e.do(http.MethodPost, "/api/reading/progress", nil, nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/reading/progress = %d，期望 405", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow == "" {
		t.Error("405 应带 Allow 头")
	}
	requireError(t, e.get("/api/nope"), http.StatusNotFound, codeNotFound)
}
