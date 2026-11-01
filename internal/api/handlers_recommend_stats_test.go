package api

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// recommendStats 取一次看板聚合数据。
func recommendStats(t *testing.T, e *testEnv, query string) recommendStatsResponse {
	t.Helper()
	rr := e.get("/api/recommend/stats" + query)
	requireStatus(t, rr, http.StatusOK)
	return decodeBody[recommendStatsResponse](t, rr)
}

// hasInterest 判断兴趣榜里有没有某个维度的一行。
func hasInterest(rows []interestStat, dim, key string) bool {
	for _, r := range rows {
		if r.Dim == dim && r.Key == key {
			return true
		}
	}
	return false
}

// interestName 取兴趣榜某一行的展示名（不存在时返回空串）。
func interestName(rows []interestStat, dim, key string) string {
	for _, r := range rows {
		if r.Dim == dim && r.Key == key {
			return r.Name
		}
	}
	return ""
}

// authorInterestName 取兴趣榜里第一个作者维度的展示名。
func authorInterestName(rows []interestStat) string {
	for _, r := range rows {
		if r.Dim == "author" {
			return r.Name
		}
	}
	return ""
}

func TestRecommendStatsRequiresRecStore(t *testing.T) {
	e := newEnv(t)
	requireError(t, e.get("/api/recommend/stats"), http.StatusServiceUnavailable, codeUnavailable)
}

func TestRecommendStatsSummaryAndDaily(t *testing.T) {
	e := newEnvWithRec(t)
	target := e.seedBook("看板之书", "看板作者")
	second := e.seedBook("看板之二", "看板作者二")
	setCategory(t, e, target, "武侠", "传统武侠")
	for _, kind := range []string{"click", "read"} {
		requireStatus(t, postEvents(t, e, eventBody(kind, target, "武侠")), http.StatusOK)
	}
	requireStatus(t, postEvents(t, e, eventBody("shelf", second, "")), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	if got.Summary.Days != 7 || got.Summary.Clicks != 1 || got.Summary.Reads != 1 ||
		got.Summary.ShelfAdds != 1 {
		t.Fatalf("汇总 = %+v", got.Summary)
	}
	if len(got.Daily) != 7 {
		t.Fatalf("按日行数 = %d，期望 7", len(got.Daily))
	}
	if today := got.Daily[6]; today.Clicks != 1 || today.Reads != 1 {
		t.Fatalf("今天那格 = %+v", today)
	}
}

func TestRecommendStatsInterestsAndHotBooks(t *testing.T) {
	e := newEnvWithRec(t)
	target := e.seedBook("看板兴趣之书", "看板兴趣作者")
	setCategory(t, e, target, "武侠", "传统武侠")
	requireStatus(t, postEvents(t, e, eventBody("read", target, "武侠")), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	// 兴趣榜同时含分类与作者两个维度，这里只断言「武侠」这一行进了榜。
	if !hasInterest(got.Interests, "category_l1", "武侠") {
		t.Fatalf("兴趣榜漏了 category_l1:武侠: %+v", got.Interests)
	}
	// L1 与 L2 是两条独立数据：同一个名字落在两个维度时不该合并成一行。
	if !hasInterest(got.Interests, "category_l2", "传统武侠") {
		t.Fatalf("兴趣榜漏了 category_l2:传统武侠: %+v", got.Interests)
	}
	// 分类的展示名就是分类名；作者必须换成作者名，否则看板上是「作者 1 / 3 / 2」。
	if name := interestName(got.Interests, "category_l1", "武侠"); name != "武侠" {
		t.Fatalf("分类展示名 = %q，期望与 key 相同", name)
	}
	if name := authorInterestName(got.Interests); name != "看板兴趣作者" {
		t.Fatalf("作者展示名 = %q，期望换成作者名", name)
	}
	if len(got.HotBooks) == 0 {
		t.Fatal("热门榜不应为空（刚读过一本书）")
	}
	if got.HotBooks[0].Title == "" || got.HotBooks[0].AuthorName == "" {
		t.Fatalf("热门榜缺主库补出的标题/作者: %+v", got.HotBooks[0])
	}
}

func TestRecommendStatsAuthorNameMissing(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("无主之书", "在册作者")
	// 显式带一个不存在的 authorId：维度补齐不会覆盖它，看板查不到名字 → name 留空。
	body := fmt.Sprintf(`{"events":[{"kind":"rating","bookId":%d,"authorId":999999}]}`, bookID)
	requireStatus(t, postEvents(t, e, body), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	if !hasInterest(got.Interests, "author", "999999") {
		t.Fatalf("兴趣榜漏了 author:999999: %+v", got.Interests)
	}
	// 查不到就不是「作者名是空串」：留空让前端回退 key，这里断言 omitempty 生效。
	if name := interestName(got.Interests, "author", "999999"); name != "" {
		t.Fatalf("缺失作者名应留空，得到 %q", name)
	}
}

func TestRecommendStatsDaysClampAndValidation(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("窗口之书", "窗口作者")
	if got := recommendStats(t, e, "?days=0"); len(got.Daily) != 1 || got.Summary.Days != 1 {
		t.Fatalf("days=0 应夹到 1 天，得到 daily=%d summary=%d", len(got.Daily), got.Summary.Days)
	}
	if got := recommendStats(t, e, "?days=999"); len(got.Daily) != 90 || got.Summary.Days != 90 {
		t.Fatalf("days=999 应夹到 90 天，得到 daily=%d summary=%d", len(got.Daily), got.Summary.Days)
	}
	if got := recommendStats(t, e, ""); len(got.Daily) != 14 || got.Summary.Days != 14 {
		t.Fatalf("默认应为 14 天，得到 daily=%d summary=%d", len(got.Daily), got.Summary.Days)
	}
	requireError(t, e.get("/api/recommend/stats?days=abc"), http.StatusBadRequest, codeInvalidParam)
}

func TestRecommendStatsSkipsMissingBooks(t *testing.T) {
	e := newEnvWithRec(t)
	e.seedBook("存在的书", "作者甲")
	// 埋点里可能有已被删除的书（事件不级联删）：热门榜应跳过它而不是占位。
	requireStatus(t, postEvents(t, e, eventBody("read", 999999, "")), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	for _, h := range got.HotBooks {
		if h.BookID == 999999 {
			t.Fatalf("主库缺失的书不应出现在热门榜: %+v", got.HotBooks)
		}
	}
}

func TestRecommendStatsWhenRecStoreClosed(t *testing.T) {
	e := newEnvWithRec(t)
	_ = e.srv.cfg.Rec.Close()
	requireError(t, e.get("/api/recommend/stats"), http.StatusInternalServerError, codeInternal)
}

func TestRecommendStatsHotBooksCap(t *testing.T) {
	e := newEnvWithRec(t)
	for i := 1; i <= 12; i++ {
		e.seedBook("热门书"+strconv.Itoa(i), "热门作者"+strconv.Itoa(i))
		// SaveBook 按 SHA 去重，但 id 与书一一对应；阅读事件打在对应 id 上。
	}
	for i := int64(1); i <= 12; i++ {
		requireStatus(t, postEvents(t, e, eventBody("read", i, "")), http.StatusOK)
	}
	got := recommendStats(t, e, "?days=7")
	if len(got.HotBooks) != statsHotLimit {
		t.Fatalf("热门榜条数 = %d，期望上限 %d", len(got.HotBooks), statsHotLimit)
	}
}
