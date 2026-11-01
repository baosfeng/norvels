package recall

import (
	"context"
	"errors"
	"testing"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 相似书召回：同系列 → 同作者 → 同分类 → 共读邻居，理由先到先得。

// seedWith 造一本带系列/作者/分类的种子书。
func seedWith(id, authorID int64, series, l1, l2 string) store.BookSummary {
	bs := book(id)
	bs.AuthorID = authorID
	bs.Series = series
	bs.CategoryL1 = l1
	bs.CategoryL2 = l2
	return bs
}

func TestSimilarReturnsErrNotFoundWhenSeedMissing(t *testing.T) {
	p := &fakePort{}
	_, err := Similar(context.Background(), p, &fakeRec{}, 99)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("种子书不存在应返回 ErrNotFound，实际 %v", err)
	}
}

func TestSimilarPropagatesSeedLookupError(t *testing.T) {
	p := &fakePort{byIDsFn: func(context.Context, []int64) ([]store.BookSummary, error) {
		return nil, errFake
	}}
	if _, err := Similar(context.Background(), p, &fakeRec{}, 99); !errors.Is(err, errFake) {
		t.Fatalf("种子书查询失败应透传，实际 %v", err)
	}
}

func TestSimilarCollectsFourPathsWithReasons(t *testing.T) {
	seed := seedWith(1, 7, "系列甲", "武侠", "江湖")
	p := fourPathPort(seed)
	r := &fakeRec{cooccurBook: func(context.Context, int64, int) ([]recstore.Cooccur, error) {
		return []recstore.Cooccur{{OtherID: 1}, {OtherID: 4, Users: 2}}, nil
	}}
	res, err := Similar(context.Background(), p, r, 1)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	assertSimilarOrder(t, res)
	assertSimilarReasons(t, res)
}

// fourPathPort 造一个「四路都能命中」的假主库：种子 1、同系列 2、同作者 3、共读 4。
func fourPathPort(seed store.BookSummary) *fakePort {
	p := &fakePort{
		byIDsFn: func(_ context.Context, ids []int64) ([]store.BookSummary, error) {
			return booksFor(ids, seed), nil
		},
	}
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		return pageFor(q, seed), nil
	}
	return p
}

// booksFor 按 id 给书目：种子 1 与共读邻居 4 有实体，其余（2 / 3）由列表路直接带上。
func booksFor(ids []int64, seed store.BookSummary) []store.BookSummary {
	if len(ids) != 1 {
		return nil
	}
	switch ids[0] {
	case seed.ID:
		return []store.BookSummary{seed}
	case 4:
		return []store.BookSummary{book(4)}
	}
	return nil
}

// pageFor 按查询条件给一页书：同系列 / 同作者 / 同 L1 分类各命中一本。
func pageFor(q store.BookQuery, seed store.BookSummary) store.Page[store.BookSummary] {
	switch {
	case q.Series == seed.Series && q.Series != "":
		return store.Page[store.BookSummary]{Items: []store.BookSummary{book(2), seed}}
	case q.AuthorID == seed.AuthorID && q.AuthorID > 0:
		return store.Page[store.BookSummary]{Items: []store.BookSummary{book(3)}}
	case q.CategoryL1 == seed.CategoryL1 && q.CategoryL1 != "":
		return store.Page[store.BookSummary]{Items: []store.BookSummary{book(3)}}
	}
	return store.Page[store.BookSummary]{}
}

// assertSimilarOrder 断言召回顺序：种子书自己排除，同分类的 3 已在同作者路收过、不重复。
func assertSimilarOrder(t *testing.T, res *Result) {
	t.Helper()
	order := idsOf(res)
	if len(order) != 3 || order[0] != 2 || order[1] != 3 || order[2] != 4 {
		t.Fatalf("相似召回顺序不对: %v", order)
	}
	if _, ok := res.Books[1]; ok {
		t.Fatal("种子书不该推荐给自己")
	}
}

// assertSimilarReasons 断言理由：相似路（同系列/同作者/同分类）的候选带文案与 code；
// 共读邻居不带（它的理由走打分的 cooccur 分量，不是关系文案）。
func assertSimilarReasons(t *testing.T, res *Result) {
	t.Helper()
	for _, c := range res.Candidates(Signals{}) {
		if c.CooccurUsers > 0 && !c.Similar {
			continue // 共读邻居
		}
		if c.SimilarReason == "" || c.SimilarReasonCode == "" || !c.Similar {
			t.Fatalf("相似候选应带理由与 code: %+v", c)
		}
	}
}

func TestSimilarSkipsEmptySeedDimensions(t *testing.T) {
	seed := book(1) // 无系列、无作者、无分类
	p := &fakePort{}
	p.byIDsFn = func(_ context.Context, ids []int64) ([]store.BookSummary, error) {
		if len(ids) == 1 && ids[0] == 1 {
			return []store.BookSummary{seed}, nil
		}
		return nil, nil
	}
	p.listFn = func(context.Context, store.BookQuery) (store.Page[store.BookSummary], error) {
		t.Fatal("种子书没有任何维度时不该查列表")
		return store.Page[store.BookSummary]{}, nil
	}
	res, err := Similar(context.Background(), p, &fakeRec{}, 1)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	if len(res.Books) != 0 {
		t.Fatalf("应召回到空结果，实际 %v", idsOf(res))
	}
}

func TestSimilarPropagatesPathErrors(t *testing.T) {
	seed := seedWith(1, 7, "系列甲", "武侠", "江湖")
	cases := []struct {
		name string
		p    *fakePort
		r    *fakeRec
		want error
	}{
		{"系列查询失败", &fakePort{listFn: func(context.Context, store.BookQuery) (store.Page[store.BookSummary], error) {
			return store.Page[store.BookSummary]{}, errFake
		}}, &fakeRec{}, errFake},
		{"共读查询失败", &fakePort{}, &fakeRec{cooccurBook: func(context.Context, int64, int) ([]recstore.Cooccur, error) {
			return nil, errFake
		}}, errFake},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.byIDsFn = func(_ context.Context, ids []int64) ([]store.BookSummary, error) {
				if len(ids) == 1 && ids[0] == 1 {
					return []store.BookSummary{seed}, nil
				}
				return nil, nil
			}
			_, err := Similar(context.Background(), tc.p, tc.r, 1)
			if !errors.Is(err, tc.want) {
				t.Fatalf("应透传底层错误，实际 %v", err)
			}
		})
	}
}

func TestSimilarTagsAreLoaded(t *testing.T) {
	seed := seedWith(1, 7, "系列甲", "", "")
	p := &fakePort{}
	p.byIDsFn = func(_ context.Context, ids []int64) ([]store.BookSummary, error) {
		if len(ids) == 1 && ids[0] == 1 {
			return []store.BookSummary{seed}, nil
		}
		return nil, nil
	}
	p.listFn = func(_ context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
		if q.Series == "系列甲" {
			return store.Page[store.BookSummary]{Items: []store.BookSummary{book(2)}}, nil
		}
		return store.Page[store.BookSummary]{}, nil
	}
	p.tagsFn = func(_ context.Context, ids []int64) (map[int64][]string, error) {
		return map[int64][]string{2: {"武侠"}}, nil
	}
	res, err := Similar(context.Background(), p, &fakeRec{}, 1)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	if got := res.Candidates(Signals{})[0].Tags; len(got) != 1 || got[0] != "武侠" {
		t.Fatalf("相似候选也应带标签，实际 %v", got)
	}
}

func TestSimilarTagErrorPropagates(t *testing.T) {
	seed := seedWith(1, 7, "系列甲", "", "")
	p := &fakePort{}
	p.byIDsFn = func(_ context.Context, ids []int64) ([]store.BookSummary, error) {
		if len(ids) == 1 && ids[0] == 1 {
			return []store.BookSummary{seed}, nil
		}
		return nil, nil
	}
	p.listFn = func(context.Context, store.BookQuery) (store.Page[store.BookSummary], error) {
		return store.Page[store.BookSummary]{Items: []store.BookSummary{book(2)}}, nil
	}
	p.tagsFn = func(context.Context, []int64) (map[int64][]string, error) { return nil, errFake }
	if _, err := Similar(context.Background(), p, &fakeRec{}, 1); !errors.Is(err, errFake) {
		t.Fatalf("标签查询失败应透传，实际 %v", err)
	}
}
