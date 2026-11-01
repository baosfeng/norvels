package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// 本文件覆盖 task-10：相似书推荐、作者级拉黑、看板理由/分类分布。

func TestRecommendSimilar(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("种子书", "作者甲")
	sibling := e.seedBook("同系列书", "作者乙")
	e.seedBook("无关书", "作者丙")
	body, err := json.Marshal(map[string]any{"series": "山河"})
	if err != nil {
		t.Fatalf("拼 PATCH: %v", err)
	}
	requireStatus(t, e.patch(bookPath(seed), string(body)), http.StatusOK)
	requireStatus(t, e.patch(bookPath(sibling), string(body)), http.StatusOK)

	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&limit=10")
	requireStatus(t, rr, http.StatusOK)
	items := decodeBody[recommendResponse](t, rr).Items
	if containsBook(items, seed) {
		t.Fatalf("相似书不应包含种子书自己: %+v", items)
	}
	found := false
	for _, it := range items {
		if it.BookID == sibling {
			found = true
			if it.Reason != "同系列《山河》" {
				t.Fatalf("同系列理由 = %q", it.Reason)
			}
			if want := bookAuthorID(t, e, sibling); it.AuthorID != want {
				t.Fatalf("推荐卡 authorId = %d，期望 %d（前端「不喜欢这个作者」要用它）", it.AuthorID, want)
			}
		}
	}
	if !found {
		t.Fatalf("同系列的书应被召回: %+v", items)
	}
}

func TestRecommendSimilarSameAuthor(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("种子甲", "同一位作者")
	sibling := e.seedBook("同作者书", "同一位作者")
	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&limit=10")
	requireStatus(t, rr, http.StatusOK)
	for _, it := range decodeBody[recommendResponse](t, rr).Items {
		if it.BookID == sibling {
			if it.Reason != "同一个作者" {
				t.Fatalf("同作者理由 = %q", it.Reason)
			}
			return
		}
	}
	t.Fatal("同作者的书应被召回")
}

func TestRecommendSimilarSameCategory(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("分类种子", "作者甲")
	sibling := e.seedBook("同类书", "作者乙")
	setCategory(t, e, seed, "武侠", "")
	setCategory(t, e, sibling, "武侠", "")

	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&limit=10")
	requireStatus(t, rr, http.StatusOK)
	for _, it := range decodeBody[recommendResponse](t, rr).Items {
		if it.BookID == sibling {
			if it.Reason != "同一分类" {
				t.Fatalf("同类理由 = %q", it.Reason)
			}
			return
		}
	}
	t.Fatal("同分类的书应被召回")
}

// L2 分类的相似召回：L1 为空时仍要按小分类找到同类书。
func TestRecommendSimilarByCategoryL2(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("二级分类种子", "作者甲")
	sibling := e.seedBook("二级同类书", "作者乙")
	setCategory(t, e, seed, "", "江湖")
	setCategory(t, e, sibling, "", "江湖")

	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&limit=10")
	requireStatus(t, rr, http.StatusOK)
	if !containsBook(decodeBody[recommendResponse](t, rr).Items, sibling) {
		t.Fatal("同 L2 分类的书应被召回")
	}
}

func TestRecommendSimilarByCooccur(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("共读种子", "作者甲")
	neighbor := e.seedBook("共读邻居", "作者乙")
	// 同一个账号把两本都读了 → 共读表里生成双向邻居对。
	requireStatus(t, postEvents(t, e, eventBody("read", seed, "")), http.StatusOK)
	requireStatus(t, postEvents(t, e, eventBody("read", neighbor, "")), http.StatusOK)

	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&limit=10")
	requireStatus(t, rr, http.StatusOK)
	items := decodeBody[recommendResponse](t, rr).Items
	if !containsBook(items, neighbor) {
		t.Fatalf("共读邻居应被召回: %+v", items)
	}
	// 同一个账号读了两本书，作者兴趣与共读同时命中；理由取贡献更大的一路，
	// 这里只断言「召回到了」，不断言具体文案（作者的先验本来就比共读强）。
}

func TestRecommendSimilarValidation(t *testing.T) {
	e := newEnvWithRec(t)
	requireError(t, e.get("/api/recommend?similar=abc"), http.StatusBadRequest, codeInvalidParam)
	requireError(t, e.get("/api/recommend?similar=999999"), http.StatusNotFound, codeNotFound)
}

func TestRecommendSimilarIgnoresTab(t *testing.T) {
	e := newEnvWithRec(t)
	seed := e.seedBook("忽略 tab 的书", "作者甲")
	rr := e.get("/api/recommend?similar=" + strconv.FormatInt(seed, 10) + "&tab=nope")
	requireStatus(t, rr, http.StatusOK)
}

func TestRecommendDislikeAuthorHidesAuthorBooks(t *testing.T) {
	e := newEnvWithRec(t)
	first := e.seedBook("拉黑甲", "被拉黑的作者")
	second := e.seedBook("拉黑乙", "被拉黑的作者")
	e.seedBook("保留丙", "别的作者")
	authorID := bookAuthorID(t, e, first)

	body, err := json.Marshal(map[string]any{
		"events": []map[string]any{{
			"kind": "dislike", "bookId": first, "authorId": authorID,
		}},
	})
	if err != nil {
		t.Fatalf("拼事件: %v", err)
	}
	requireStatus(t, postEvents(t, e, string(body)), http.StatusOK)

	items := recommendItems(t, e, 10)
	if len(items) == 0 {
		t.Fatal("还有别的作者的书，推荐不应为空")
	}
	for _, it := range items {
		if it.BookID == first || it.BookID == second {
			t.Fatalf("被拉黑作者的书不应出现: %+v", items)
		}
	}
}

func TestStatsReasonMixAndCategoryClicks(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("分布之书", "分布作者")
	setCategory(t, e, bookID, "武侠", "")
	events := []map[string]any{
		{"kind": "click", "bookId": bookID, "categoryL1": "武侠", "extra": "因为你常看武侠"},
		{"kind": "click", "bookId": bookID, "categoryL1": "武侠", "extra": "因为你常看武侠"},
		{"kind": "click", "bookId": bookID, "categoryL1": "武侠", "extra": "随机发现"},
	}
	body, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatalf("拼事件: %v", err)
	}
	requireStatus(t, postEvents(t, e, string(body)), http.StatusOK)

	got := recommendStats(t, e, "?days=7")
	if len(got.ReasonMix) != 2 {
		t.Fatalf("理由分布 = %+v，期望 2 条", got.ReasonMix)
	}
	// 历史行里 extra 存的是文案：看板要按稳定 code 分组、文案单独给，
	// 否则改一句措辞就会把同一类理由拆成两行。
	if got.ReasonMix[0].Reason != "interest:武侠" || got.ReasonMix[0].Clicks != 2 {
		t.Fatalf("理由榜首 = %+v", got.ReasonMix[0])
	}
	if got.ReasonMix[0].Label != "因为你常看武侠" {
		t.Fatalf("榜首文案 = %q", got.ReasonMix[0].Label)
	}
	if got.ReasonMix[1].Reason != "random" {
		t.Fatalf("第二行应归一化成随机发现的 code，得到 %+v", got.ReasonMix[1])
	}
	if len(got.CategoryClicks) != 1 || got.CategoryClicks[0].CategoryL1 != "武侠" ||
		got.CategoryClicks[0].Clicks != 3 {
		t.Fatalf("分类点击 = %+v", got.CategoryClicks)
	}
}

// 推荐卡必须带 authorId：前端「不喜欢这个作者」直接下发 dislike，
// 只给 authorName 的话还得先查详情才能构造事件。
func TestRecommendItemsCarryAuthorID(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("带作者的书", "带作者")
	want := bookAuthorID(t, e, bookID)

	items := recommendItems(t, e, 10)
	if len(items) == 0 {
		t.Fatal("推荐不应为空")
	}
	for _, it := range items {
		if it.BookID == bookID {
			if it.AuthorID != want {
				t.Fatalf("authorId = %d，期望 %d", it.AuthorID, want)
			}
			return
		}
	}
	t.Fatalf("推荐里应包含刚入库的书: %+v", items)
}
