package recall

import (
	"context"
	"errors"
	"testing"
	"time"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 测试口径：召回 = 「按 tab 调哪些路 + 每路怎么把库结果收进来」。
// 断言的顺序与内容都必须确定（同一份输入两次跑必须一模一样）。

func TestCandidatesTabDispatchesPaths(t *testing.T) {
	cases := []struct {
		name string
		tab  string
		// wantList / wantHot / wantProgress 断言该 tab 会不会去查对应的库。
		wantList     bool
		wantHot      bool
		wantProgress bool
	}{
		{"默认 tab 走全量九路", TabForYou, true, true, true},
		{"未知 tab 回退默认全量", "wat", true, true, true},
		{"续读 tab 只查进度 + 热度兜底", TabResume, true, true, true},
		{"随机 tab 只查随机池 + 热度兜底", TabRandom, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePort{}
			r := &fakeRec{}
			mustCandidates(t, p, r, nil, tc.tab)
			if got := len(p.listCalls) > 0; got != tc.wantList {
				t.Fatalf("ListBooks 调用 = %v，期望 %v", got, tc.wantList)
			}
		})
	}
}

func TestCandidatesTabRandomSkipsInterestPaths(t *testing.T) {
	p := &fakePort{}
	r := &fakeRec{}
	interest := map[string]float64{"category_l1:武侠": 5, "author:3": 4}
	mustCandidates(t, p, r, interest, TabRandom)
	if len(p.listCalls) != 0 {
		t.Fatalf("随机 tab 不该按兴趣查列表，实际查询 %d 次: %+v", len(p.listCalls), p.listCalls)
	}
}

func TestCandidatesCollectsAllPaths(t *testing.T) {
	p := &fakePort{books: map[int64]store.BookSummary{
		1: book(1), 2: book(2), 3: book(3), 4: book(4), 5: book(5), 8: book(8), 9: book(9),
	}}
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		switch {
		case q.CategoryL1 == "武侠":
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(1)}}, nil
		case q.Series == "系列甲":
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(2)}}, nil
		case q.Sort == "imported":
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(3)}}, nil
		}
		return store.Page[store.BookSummary]{}, nil
	}
	p.randomFn = func(context.Context, int) ([]store.BookSummary, error) {
		return []store.BookSummary{book(4)}, nil
	}
	p.progFn = func(context.Context, int) ([]store.ReadingProgress, error) {
		return []store.ReadingProgress{
			{BookID: 5, Percent: 0.5}, // 读到一半 → 续读候选
			{BookID: 6, Percent: 0},   // 没开始 → 排除
			{BookID: 7, Percent: 1},   // 读完了 → 排除
		}, nil
	}
	r := &fakeRec{
		cooccurFn: func(context.Context, int64, int) ([]recstore.Cooccur, error) {
			return []recstore.Cooccur{{OtherID: 8, Users: 5}, {OtherID: 8, Users: 9}}, nil
		},
		hotFn: func(context.Context, int, int) ([]recstore.HotBook, error) {
			return []recstore.HotBook{{BookID: 9, Reads: 30, Clicks: 3}}, nil
		},
	}
	interest := map[string]float64{"category_l1:武侠": 9, "series:系列甲": 4}
	res := mustCandidates(t, p, r, interest, TabForYou)

	for _, id := range []int64{1, 2, 3, 4, 5, 8, 9} {
		if _, ok := res.Books[id]; !ok {
			t.Fatalf("书 %d 应被召回，实际 order=%v", id, idsOf(res))
		}
	}
	for _, id := range []int64{6, 7} {
		if _, ok := res.Books[id]; ok {
			t.Fatalf("书 %d（进度收尾）不该被召回", id)
		}
	}
	if res.Resume[5] != 0.5 {
		t.Fatalf("续读进度应记 0.5，实际 %v", res.Resume[5])
	}
}

func TestCandidatesDeduplicatesAndKeepsOrder(t *testing.T) {
	p := &fakePort{}
	// 三路都给同一本 1：只应收一次，且保持第一次出现的顺序。
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		if q.Sort == "imported" {
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(1), book(2)}}, nil
		}
		return store.Page[store.BookSummary]{Items: []store.BookSummary{book(1)}}, nil
	}
	interest := map[string]float64{"category_l1:武侠": 9}
	res := mustCandidates(t, p, &fakeRec{}, interest, TabForYou)
	order := idsOf(res)
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("去重保序失败: %v", order)
	}
}

func TestCandidatesSkipsZeroAndNegativeBookID(t *testing.T) {
	p := &fakePort{}
	p.randomFn = func(context.Context, int) ([]store.BookSummary, error) {
		return []store.BookSummary{{ID: 0}, {ID: -3}, book(11)}, nil
	}
	res := mustCandidates(t, p, &fakeRec{}, nil, TabRandom)
	if len(res.Books) != 1 || res.Books[11].ID != 11 {
		t.Fatalf("非法 id 应被丢弃，剩 %v", idsOf(res))
	}
}

func TestCandidatesLoadsTagsForEveryCandidate(t *testing.T) {
	p := &fakePort{}
	p.randomFn = func(context.Context, int) ([]store.BookSummary, error) {
		return []store.BookSummary{book(1)}, nil
	}
	var asked []int64
	p.tagsFn = func(_ context.Context, ids []int64) (map[int64][]string, error) {
		asked = append(asked, ids...)
		return map[int64][]string{1: {"武侠"}}, nil
	}
	res := mustCandidates(t, p, &fakeRec{}, nil, TabRandom)
	if len(asked) != 1 || asked[0] != 1 {
		t.Fatalf("标签应一次性按候选补齐，实际问过 %v", asked)
	}
	if got := res.Candidates(Signals{})[0].Tags; len(got) != 1 || got[0] != "武侠" {
		t.Fatalf("候选应带上标签，实际 %v", got)
	}
}

func TestCandidatesSkipsTagQueryWhenEmpty(t *testing.T) {
	p := &fakePort{}
	p.tagsFn = func(context.Context, []int64) (map[int64][]string, error) {
		t.Fatal("没有候选时不该查标签")
		return nil, nil
	}
	res := mustCandidates(t, p, &fakeRec{}, nil, TabRandom)
	if len(res.Books) != 0 {
		t.Fatalf("空库应召回到空结果，实际 %v", idsOf(res))
	}
}

func TestCandidatesPropagatesErrors(t *testing.T) {
	cases := []struct {
		name string
		p    *fakePort
		r    *fakeRec
	}{
		{"兴趣分类查询失败", &fakePort{listFn: func(context.Context, store.BookQuery) (store.Page[store.BookSummary], error) {
			return store.Page[store.BookSummary]{}, errFake
		}}, &fakeRec{}},
		{"随机池查询失败", &fakePort{randomFn: func(context.Context, int) ([]store.BookSummary, error) {
			return nil, errFake
		}}, &fakeRec{}},
		{"进度查询失败", &fakePort{progFn: func(context.Context, int) ([]store.ReadingProgress, error) {
			return nil, errFake
		}}, &fakeRec{}},
		{"热度查询失败", &fakePort{}, &fakeRec{hotFn: func(context.Context, int, int) ([]recstore.HotBook, error) {
			return nil, errFake
		}}},
		{"共读查询失败", &fakePort{}, &fakeRec{cooccurFn: func(context.Context, int64, int) ([]recstore.Cooccur, error) {
			return nil, errFake
		}}},
		{"标签查询失败", &fakePort{randomFn: func(context.Context, int) ([]store.BookSummary, error) {
			return []store.BookSummary{book(1)}, nil
		}, tagsFn: func(context.Context, []int64) (map[int64][]string, error) {
			return nil, errFake
		}}, &fakeRec{}},
		{"按 id 补书目失败", &fakePort{byIDsFn: func(context.Context, []int64) ([]store.BookSummary, error) {
			return nil, errFake
		}}, &fakeRec{hotFn: func(context.Context, int, int) ([]recstore.HotBook, error) {
			return []recstore.HotBook{{BookID: 1}}, nil
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Candidates(context.Background(), tc.p, tc.r, 7, nil, TabForYou); !errors.Is(err, errFake) {
				t.Fatalf("应把底层错误透传出来，实际 %v", err)
			}
		})
	}
}

func TestCandidatesCategoryFallsBackToL2(t *testing.T) {
	p := &fakePort{}
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		if q.CategoryL2 == "小分类" {
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(21)}}, nil
		}
		return store.Page[store.BookSummary]{}, nil
	}
	res := mustCandidates(t, p, &fakeRec{}, map[string]float64{"category_l1:小分类": 9}, TabForYou)
	if _, ok := res.Books[21]; !ok {
		t.Fatalf("L1 没命中时必须回退查 L2，实际 %v", idsOf(res))
	}
	// 先 L1 后 L2 的顺序让大分类命中时少一次查询。
	if len(p.listCalls) < 2 || p.listCalls[0].CategoryL1 != "小分类" || p.listCalls[1].CategoryL2 != "小分类" {
		t.Fatalf("L1 → L2 的查询顺序不对: %+v", p.listCalls)
	}
}

func TestCandidatesAuthorKeyIgnoresDirtyValue(t *testing.T) {
	p := &fakePort{}
	var authorQuery int64
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		if q.AuthorID > 0 {
			authorQuery = q.AuthorID
		}
		return store.Page[store.BookSummary]{}, nil
	}
	// 非数字的作者 key 是脏数据：跳过，不影响其它作者。
	res := mustCandidates(t, p, &fakeRec{}, map[string]float64{"author:脏值": 9, "author:42": 8}, TabForYou)
	if authorQuery != 42 {
		t.Fatalf("合法作者 key 应照常召回，实际查询 authorID=%d", authorQuery)
	}
	_ = res
}

func TestCandidatesSignalsFlowIntoScoringInput(t *testing.T) {
	p := &fakePort{}
	p.randomFn = func(context.Context, int) ([]store.BookSummary, error) {
		return []store.BookSummary{book(31)}, nil
	}
	p.tagsFn = func(context.Context, []int64) (map[int64][]string, error) {
		return map[int64][]string{31: {"武侠"}}, nil
	}
	res := mustCandidates(t, p, &fakeRec{}, nil, TabRandom)
	now := time.Now().UTC()
	sig := Signals{
		Read:     map[int64]bool{31: true},
		Finished: map[int64]bool{31: true},
		Recent:   map[int64]bool{31: true},
		Now:      now,
	}
	cands := res.Candidates(sig)
	if len(cands) != 1 {
		t.Fatalf("应产出 1 个候选，实际 %d", len(cands))
	}
	c := cands[0]
	if !c.Read || !c.Finished || !c.RecentShown {
		t.Fatalf("读状态信号没传进候选: %+v", c)
	}
	if len(c.Tags) != 1 || c.Tags[0] != "武侠" {
		t.Fatalf("标签没传进候选: %+v", c.Tags)
	}
}
