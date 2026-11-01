package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// 本文件覆盖 task-8 的四块能力：dislike 负反馈、续读召回、标签/系列兴趣、tab 参数。

func TestRecommendTabInvalid(t *testing.T) {
	e := newEnvWithRec(t)
	requireError(t, e.get("/api/recommend?tab=nope"), http.StatusBadRequest, codeInvalidParam)
}

func TestRecommendExcludesDislikedBook(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("没被讨厌的书", "作者甲")
	disliked := e.seedBook("被讨厌的书", "作者乙")
	setCategory(t, e, disliked, "武侠", "")
	requireStatus(t, postEvents(t, e, eventBody("dislike", disliked, "武侠")), http.StatusOK)

	items := recommendItems(t, e, 10)
	if len(items) == 0 {
		t.Fatal("剔除一本后仍有候选，推荐不应为空")
	}
	for _, it := range items {
		if it.BookID == disliked {
			t.Fatalf("被 dislike 的书不应再出现: %+v", items)
		}
	}
}

// saveProgress 通过 API 写一条阅读进度（续读召回的数据来源）。
func saveProgress(t *testing.T, e *testEnv, bookID, chapterID int64, percent float64) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"bookId": bookID, "chapterId": chapterID, "percent": percent,
	})
	if err != nil {
		t.Fatalf("拼进度请求: %v", err)
	}
	requireStatus(t, e.do(http.MethodPut, "/api/reading/progress",
		strings.NewReader(string(body)), jsonHeader), http.StatusNoContent)
}

func TestRecommendResumeTab(t *testing.T) {
	e := newEnvWithRec(t)
	early := e.seedBook("刚开头的书", "作者甲")
	late := e.seedBook("快读完的书", "作者乙")
	saveProgress(t, e, early, firstChapterID(t, e, early), 0.2)
	saveProgress(t, e, late, firstChapterID(t, e, late), 0.8)

	rr := e.get("/api/recommend?tab=resume&limit=10")
	requireStatus(t, rr, http.StatusOK)
	items := decodeBody[recommendResponse](t, rr).Items
	if len(items) < 2 {
		t.Fatalf("续读 tab 应返回两本在读的书，得到 %+v", items)
	}
	// 按进度倒序：0.8 那本在前，且理由固定为续读。
	if items[0].BookID != late || items[1].BookID != early {
		t.Fatalf("续读顺序 = %+v，期望 [%d %d]", items, late, early)
	}
	if items[0].Reason != "读到一半，接着读" {
		t.Fatalf("续读理由 = %q", items[0].Reason)
	}
}

func TestRecommendRandomTabExploresHalf(t *testing.T) {
	e := newEnvWithRec(t)
	for i := 0; i < 12; i++ {
		e.seedBook("随机池书"+strconv.Itoa(i), "随机作者"+strconv.Itoa(i))
	}
	rr := e.get("/api/recommend?tab=random&limit=10")
	requireStatus(t, rr, http.StatusOK)
	items := decodeBody[recommendResponse](t, rr).Items
	if len(items) != 10 {
		t.Fatalf("随机 tab 条数 = %d，期望 10", len(items))
	}
	random := 0
	for _, it := range items {
		if it.Reason == "随机发现" {
			random++
		}
	}
	if random != 5 {
		t.Fatalf("随机 tab 的探索比例应为 50%%（5 条），得到 %d: %+v", random, items)
	}
}

func TestRecommendStatsTagAndSeriesDims(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("标签系列之书", "标签系列作者")
	body, err := json.Marshal(map[string]any{"tags": []string{"武侠"}, "series": "山河"})
	if err != nil {
		t.Fatalf("拼 PATCH 请求: %v", err)
	}
	requireStatus(t, e.patch(bookPath(bookID), string(body)), http.StatusOK)
	requireStatus(t, postEvents(t, e, eventBody("click", bookID, "")), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	cases := [][2]string{{"tag", "武侠"}, {"series", "山河"}}
	for _, c := range cases {
		dim, key := c[0], c[1]
		if !hasInterest(got.Interests, dim, key) {
			t.Fatalf("兴趣榜缺 %s:%s: %+v", dim, key, got.Interests)
		}
		if name := interestName(got.Interests, dim, key); name != key {
			t.Fatalf("%s:%s 的展示名 = %q，期望与 key 相同", dim, key, name)
		}
	}
}

func TestRecommendTagsAndSeriesRecall(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("标签召回之书", "标签召回作者")
	body, err := json.Marshal(map[string]any{"tags": []string{"武侠"}, "series": "山河"})
	if err != nil {
		t.Fatalf("拼 PATCH 请求: %v", err)
	}
	requireStatus(t, e.patch(bookPath(bookID), string(body)), http.StatusOK)
	// 事件只带 bookId：标签与系列由服务端补齐，随后作为兴趣维度进入召回。
	requireStatus(t, postEvents(t, e, eventBody("click", bookID, "")), http.StatusOK)

	items := recommendItems(t, e, 10)
	if !containsBook(items, bookID) {
		t.Fatalf("标签/系列召回应把这本书带回来: %+v", items)
	}
}

func TestRecommendResumeTabSamePercentKeepsScoreOrder(t *testing.T) {
	e := newEnvWithRec(t)
	first := e.seedBook("并排甲", "作者甲")
	second := e.seedBook("并排乙", "作者乙")
	saveProgress(t, e, first, firstChapterID(t, e, first), 0.5)
	saveProgress(t, e, second, firstChapterID(t, e, second), 0.5)

	rr := e.get("/api/recommend?tab=resume&limit=5")
	requireStatus(t, rr, http.StatusOK)
	if items := decodeBody[recommendResponse](t, rr).Items; len(items) < 2 {
		t.Fatalf("进度相同时也应都返回: %+v", items)
	}
}

func TestEventsTagValidation(t *testing.T) {
	e := newEnvWithRec(t)
	long := strings.Repeat("x", maxEventFieldLen+1)
	tooMany := make([]string, maxEventTags+1)
	for i := range tooMany {
		tooMany[i] = "标签" + strconv.Itoa(i)
	}
	bookID := e.seedBook("标签校验之书", "标签校验作者")
	cases := map[string]string{
		"标签过多": fmt.Sprintf(`{"events":[{"kind":"click","bookId":%d,"tags":[%s]}]}`, bookID, quotedList(tooMany)),
		"标签过长": fmt.Sprintf(`{"events":[{"kind":"click","bookId":%d,"tags":["%s"]}]}`, bookID, long),
		"系列过长": fmt.Sprintf(`{"events":[{"kind":"click","bookId":%d,"series":"%s"}]}`, bookID, long),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireError(t, postEvents(t, e, body), http.StatusBadRequest, codeInvalidParam)
		})
	}
}

// quotedList 把字符串切片拼成 JSON 数组里的多个字符串字面量。
func quotedList(items []string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = strconv.Quote(it)
	}
	return strings.Join(parts, ",")
}

func TestRecommendTabDefaultIsForYou(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("默认 tab 之书", "作者甲")
	items := recommendItems(t, e, 5)
	if len(items) == 0 {
		t.Fatal("默认 tab 应返回非空推荐")
	}
}
