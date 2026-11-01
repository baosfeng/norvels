package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 本文件补齐召回各路的覆盖：作者召回、小分类回退、共读召回、多候选与截断。
// 这些都是「有数据才走得到」的分支，冷启动用例碰不着。

func TestRecommendAuthorRecall(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("作者召回之书", "召回作者")
	authorID := bookAuthorID(t, e, bookID)
	body := `{"events":[{"kind":"rating","bookId":` + strconv.FormatInt(bookID, 10) +
		`,"authorId":` + strconv.FormatInt(authorID, 10) + `}]}`
	requireStatus(t, postEvents(t, e, body), http.StatusOK)

	if !containsBook(recommendItems(t, e, 10), bookID) {
		t.Fatal("作者兴趣召回的书应出现在推荐里")
	}
}

func TestRecommendCategoryL2Fallback(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("小分类之书", "小分类作者")
	setCategory(t, e, bookID, "大分类", "小分类")
	// 兴趣 key 只存分类名：「小分类」不是任何书的 L1，必须回退到 L2 才召回得到。
	requireStatus(t, postEvents(t, e, eventBody("click", bookID, "小分类")), http.StatusOK)
	if !containsBook(recommendItems(t, e, 10), bookID) {
		t.Fatal("小分类应通过 L2 回退被召回")
	}
}

func TestRecommendCooccurRecall(t *testing.T) {
	e := newEnvWithRec(t)
	first := e.seedBook("共读甲", "作者甲")
	second := e.seedBook("共读乙", "作者乙")
	// 另一个账号把两本都读了 → 生成共读对；当前账号只读第一本，第二本成为共读邻居。
	ctx := context.Background()
	if _, err := e.srv.cfg.Rec.RecordEvents(ctx, 99, []recstore.Event{
		{Kind: recstore.KindRead, BookID: first},
		{Kind: recstore.KindRead, BookID: second},
	}); err != nil {
		t.Fatalf("写入共读样本: %v", err)
	}
	requireStatus(t, postEvents(t, e, eventBody("read", first, "")), http.StatusOK)

	if !containsBook(recommendItems(t, e, 10), second) {
		t.Fatal("共读邻居应出现在推荐里")
	}
}

func TestRecommendRankPoolWhenManyCandidates(t *testing.T) {
	e := newEnvWithRec(t)
	for i := 0; i < 10; i++ {
		e.seedBook(fmt.Sprintf("批量测试书%d", i), fmt.Sprintf("批量作者%d", i))
	}
	if items := recommendItems(t, e, 2); len(items) != 2 {
		t.Fatalf("limit=2 应返回 2 条，得到 %d", len(items))
	}
}

func TestRecommendTopCategoriesAreTruncated(t *testing.T) {
	e := newEnvWithRec(t)
	for i := 0; i < 5; i++ {
		id := e.seedBook(fmt.Sprintf("分类书%d", i), fmt.Sprintf("分类作者%d", i))
		setCategory(t, e, id, fmt.Sprintf("分类%d", i), "")
		requireStatus(t, postEvents(t, e, eventBody("click", id, fmt.Sprintf("分类%d", i))), http.StatusOK)
	}
	if items := recommendItems(t, e, 10); len(items) == 0 {
		t.Fatal("多分类兴趣下推荐不应为空")
	}
}

func TestRecommendAndEventsWhenRecStoreClosed(t *testing.T) {
	e := newEnvWithRec(t)
	_ = e.srv.cfg.Rec.Close()
	requireError(t, e.get("/api/recommend"), http.StatusInternalServerError, codeInternal)
	requireError(t, postEvents(t, e, eventBody("click", 1, "武侠")),
		http.StatusInternalServerError, codeInternal)
}

func TestEventsRejectsInvalidTimestamp(t *testing.T) {
	e := newEnvWithRec(t)
	// ts 交给数据层校验：api 不解释时间格式，坏值应由 recstore 判成参数错误。
	body := `{"events":[{"kind":"click","bookId":1,"ts":"昨天"}]}`
	requireError(t, postEvents(t, e, body), http.StatusBadRequest, codeInvalidParam)
}

func TestEventsBackfillBookDims(t *testing.T) {
	e := newEnvWithRec(t)
	bookID := e.seedBook("维度补齐之书", "维度作者")
	setCategory(t, e, bookID, "玄幻", "东方玄幻")
	// 客户端只带 bookId：作者与分类应由服务端按书补齐，否则兴趣聚不起来。
	body := `{"events":[{"kind":"click","bookId":` +
		strconv.FormatInt(bookID, 10) + `}]}`
	requireStatus(t, postEvents(t, e, body), http.StatusOK)

	interest, err := e.srv.cfg.Rec.Interests(context.Background(), store.ScopeUserID(context.Background()))
	if err != nil {
		t.Fatalf("读兴趣: %v", err)
	}
	if _, ok := interest["category_l1:玄幻"]; !ok {
		t.Fatalf("一级分类维度未补齐: %v", interest)
	}
	if _, ok := interest["category_l2:东方玄幻"]; !ok {
		t.Fatalf("二级分类维度未补齐: %v", interest)
	}
}

func TestRecommendWhenMainStoreClosed(t *testing.T) {
	e := newEnvWithRec(t)
	// 推荐库正常、主库挂了：召回查主库失败要走 500，而不是静默返回空列表。
	_ = e.st.Close()
	requireError(t, e.get("/api/recommend"), http.StatusInternalServerError, codeInternal)
}

// containsBook 判断推荐结果里有没有这本书。
func containsBook(items []recommendItem, bookID int64) bool {
	for _, it := range items {
		if it.BookID == bookID {
			return true
		}
	}
	return false
}
