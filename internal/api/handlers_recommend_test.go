package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// newEnvWithRec 打开一套带推荐库的测试环境（推荐端点依赖独立的 rec 库）。
func newEnvWithRec(t *testing.T) *testEnv {
	t.Helper()
	rec, err := recstore.Open(context.Background(), filepath.Join(t.TempDir(), "norvels-rec.db"))
	if err != nil {
		t.Fatalf("recstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })
	return newEnvWith(t, Config{
		Version:          "rec",
		DisableAccessLog: true,
		Rec:              rec,
	}, testFS())
}

// recommendItems 取一次 /api/recommend 的 items。
func recommendItems(t *testing.T, e *testEnv, limit int) []recommendItem {
	t.Helper()
	rr := e.get("/api/recommend?limit=" + strconv.Itoa(limit))
	requireStatus(t, rr, http.StatusOK)
	return decodeBody[recommendResponse](t, rr).Items
}

// eventBody 拼一个单条事件的上报体（kind + bookId + 可选分类）。
func eventBody(kind string, bookID int64, l1 string) string {
	return `{"events":[{"kind":"` + kind + `","bookId":` +
		strconv.FormatInt(bookID, 10) + `,"categoryL1":"` + l1 + `"}]}`
}

func TestRecommendRequiresRecStore(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.get("/api/recommend"), http.StatusServiceUnavailable, codeUnavailable)
}

func TestRecommendLimitClampAndValidation(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("钳制之书", "作者甲")
	for _, target := range []string{
		"/api/recommend", "/api/recommend?limit=999", "/api/recommend?limit=0",
	} {
		requireStatus(t, e.get(target), http.StatusOK)
	}
	requireError(t, e.get("/api/recommend?limit=abc"), http.StatusBadRequest, codeInvalidParam)
}

func TestRecommendColdStartNotEmpty(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("冷启动甲", "作者甲")
	e.seedBook("冷启动乙", "作者乙")
	items := recommendItems(t, e, 10)
	if len(items) == 0 {
		t.Fatal("冷启动推荐不应为空")
	}
	for _, it := range items {
		if it.Title == "" || it.Reason == "" {
			t.Fatalf("推荐卡字段不全: %+v", it)
		}
	}
}

func TestRecommendInterestRecallAndReason(t *testing.T) {
	e := newEnvWithRec(t)
	target := e.seedBook("武侠之书", "武侠作者")
	setCategory(t, e, target, "武侠", "传统武侠")
	e.seedBook("都市之书", "都市作者")

	for _, kind := range []string{"click", "shelf", "rating"} {
		requireStatus(t, postEvents(t, e, eventBody(kind, target, "武侠")), http.StatusOK)
	}
	items := recommendItems(t, e, 5)
	if len(items) == 0 || items[0].BookID != target {
		t.Fatalf("兴趣分类的书应排第一，得到 %+v", items)
	}
	if items[0].Reason != "因为你常看武侠" {
		t.Fatalf("理由 = %q，期望兴趣类理由", items[0].Reason)
	}
}

func TestRecommendSkipsReadBooks(t *testing.T) {
	e := newEnvWithRec(t)
	read := e.seedBook("读过的书", "作者甲")
	other := e.seedBook("没读过的书", "作者乙")
	setCategory(t, e, other, "都市", "")

	requireStatus(t, postEvents(t, e, eventBody("read", read, "")), http.StatusOK)

	items := recommendItems(t, e, 10)
	if len(items) == 0 {
		t.Fatal("除已读书之外仍有候选，推荐不应为空")
	}
	for _, it := range items {
		if it.BookID == read {
			t.Fatalf("已读书不应被推荐: %+v", items)
		}
	}
}

func TestRecommendTwoCallsDiffer(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("随机甲", "作者甲")
	e.seedBook("随机乙", "作者乙")
	first := recommendItems(t, e, 5)
	second := recommendItems(t, e, 5)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("两次推荐都应为非空: %d / %d", len(first), len(second))
	}
	if first[0].Score == second[0].Score {
		t.Fatalf("两次请求的分数应不同（随机扰动失效）: %v", first[0].Score)
	}
}

// sessionBody 拼一条阅读会话上报体。
func sessionBody(bookID, chapterID int64, seconds int) string {
	return `{"bookId":` + strconv.FormatInt(bookID, 10) +
		`,"chapterId":` + strconv.FormatInt(chapterID, 10) +
		`,"seconds":` + strconv.Itoa(seconds) + `}`
}

func TestReadingSessionFeedsRecStore(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("会话之书", "会话作者")
	body := sessionBody(bookID, firstChapterID(t, e, bookID), 120)
	requireStatus(t, e.do(http.MethodPost, "/api/reading/sessions",
		strings.NewReader(body), jsonHeader), http.StatusNoContent)

	read, err := e.srv.cfg.Rec.ReadBooks(context.Background(), store.ScopeUserID(context.Background()))
	if err != nil || !read[bookID] {
		t.Fatalf("阅读会话应同步成 rec read 事件: %v err=%v", read, err)
	}
}

func TestReadingSessionSurvivesRecFailure(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("降级之书", "降级作者")
	chapterID := firstChapterID(t, e, bookID)
	// 推荐库关闭后仍要 204：阅读上报成功，推荐信号失败只记日志。
	_ = e.srv.cfg.Rec.Close()
	requireStatus(t, e.do(http.MethodPost, "/api/reading/sessions",
		strings.NewReader(sessionBody(bookID, chapterID, 60)), jsonHeader), http.StatusNoContent)
}
