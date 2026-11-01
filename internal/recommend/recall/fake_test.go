package recall

import (
	"context"
	"testing"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 本文件的假存储只实现 Port / RecPort：召回的逻辑全部可测，不必起 SQLite。
//
// 每个方法都有「按需注入」的函数钩子：默认返回空结果，需要精确控制查询
// 内容或错误路径的用例再设置钩子，避免为每个用例写一整套样板假实现。

type fakePort struct {
	books     map[int64]store.BookSummary
	listFn    func(context.Context, store.BookQuery) (store.Page[store.BookSummary], error)
	byIDsFn   func(context.Context, []int64) ([]store.BookSummary, error)
	tagsFn    func(context.Context, []int64) (map[int64][]string, error)
	progFn    func(context.Context, int) ([]store.ReadingProgress, error)
	randomFn  func(context.Context, int) ([]store.BookSummary, error)
	listCalls []store.BookQuery
}

func (f *fakePort) ListBooks(ctx context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
	f.listCalls = append(f.listCalls, q)
	if f.listFn != nil {
		return f.listFn(ctx, q)
	}
	return store.Page[store.BookSummary]{}, nil
}

func (f *fakePort) BooksByIDs(ctx context.Context, ids []int64) ([]store.BookSummary, error) {
	if f.byIDsFn != nil {
		return f.byIDsFn(ctx, ids)
	}
	out := make([]store.BookSummary, 0, len(ids))
	for _, id := range ids {
		if bs, ok := f.books[id]; ok {
			out = append(out, bs)
		}
	}
	return out, nil
}

func (f *fakePort) BookTagsByIDs(ctx context.Context, ids []int64) (map[int64][]string, error) {
	if f.tagsFn != nil {
		return f.tagsFn(ctx, ids)
	}
	return map[int64][]string{}, nil
}

func (f *fakePort) ListReadingProgress(ctx context.Context, limit int) ([]store.ReadingProgress, error) {
	if f.progFn != nil {
		return f.progFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakePort) RandomBookSummaries(ctx context.Context, n int) ([]store.BookSummary, error) {
	if f.randomFn != nil {
		return f.randomFn(ctx, n)
	}
	return nil, nil
}

type fakeRec struct {
	cooccurFn   func(context.Context, int64, int) ([]recstore.Cooccur, error)
	cooccurBook func(context.Context, int64, int) ([]recstore.Cooccur, error)
	hotFn       func(context.Context, int, int) ([]recstore.HotBook, error)
}

func (f *fakeRec) Cooccur(ctx context.Context, userID int64, limit int) ([]recstore.Cooccur, error) {
	if f.cooccurFn != nil {
		return f.cooccurFn(ctx, userID, limit)
	}
	return nil, nil
}

func (f *fakeRec) CooccurForBook(ctx context.Context, bookID int64, limit int) ([]recstore.Cooccur, error) {
	if f.cooccurBook != nil {
		return f.cooccurBook(ctx, bookID, limit)
	}
	return nil, nil
}

func (f *fakeRec) HotBooks(ctx context.Context, days, limit int) ([]recstore.HotBook, error) {
	if f.hotFn != nil {
		return f.hotFn(ctx, days, limit)
	}
	return nil, nil
}

// book 造一本最小书目（只有召回与打分用到的字段）。
func book(id int64) store.BookSummary {
	return store.BookSummary{ID: id, Title: "书" + string(rune('0'+id%10))}
}

// errFake 是错误路径用的哨兵错误。
var errFake = errFakeType{}

type errFakeType struct{}

func (errFakeType) Error() string { return "假存储故障" }

// mustCandidates 跑一次召回并断言没有错误。
func mustCandidates(t *testing.T, p Port, r RecPort, interest map[string]float64, tab string) *Result {
	t.Helper()
	res, err := Candidates(context.Background(), p, r, 7, interest, tab)
	if err != nil {
		t.Fatalf("Candidates(%s): %v", tab, err)
	}
	return res
}

// idsOf 返回召回顺序。
func idsOf(res *Result) []int64 { return res.order }
