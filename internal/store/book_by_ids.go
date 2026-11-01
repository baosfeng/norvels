package store

import (
	"context"
	"fmt"
	"math/rand/v2"
)

// 按 id 批量取书与随机取样（推荐召回用）。
//
// 为什么放在 store 而不是 api：召回先按「分类 / 作者 / 共读 / 热度」凑出 id，
// 再统一回主库补标题与封面——那次补全必须与列表页共用 bookSummaryColumns，
// 否则推荐卡和书架卡会各写各的字段口径。

// maxIDsPerQuery 是**单条** IN 查询的 id 上限（书 / 作者共用同一口径）。
//
// SQLite 的绑定参数上限默认 999，这里取 500 留余量。注意它只是分段粒度，
// 不是调用方的容量上限：超过就切段查询（见 idChunks），语义与不超量时完全一致。
// 为什么必须切段而不能报错——推荐的候选池由各路召回配额相加得到（当前最坏 520），
// 那是业务上完全合理的输入；把「存储层的绑定参数上限」当成「业务层的硬上限」
// 顶回去，会让用户多出几个兴趣分类就拿到 500，而错误信息还指向 sqlite。
const maxIDsPerQuery = 500

// BooksByIDs 按传入顺序返回这些书（跳过不存在 / 已被取代 / 非正 id）。
//
// 顺序由调用方决定（兴趣、共读、热度各有各的序），而 SQL 的 IN 不保证顺序，
// 所以这里按输入顺序回填。只返回 superseded_by = 0 的书：旧版本在书架上本来就不可见，
// 推荐里冒出一本被替换掉的旧版，用户点进去会看到重复书目。
func (s *Store) BooksByIDs(ctx context.Context, ids []int64) ([]BookSummary, error) {
	uniq := dedupeIDs(ids)
	if len(uniq) == 0 {
		return nil, nil
	}
	items := make([]BookSummary, 0, len(uniq))
	for _, chunk := range idChunks(uniq) {
		part, err := s.loadBooksByIDs(ctx, chunk)
		if err != nil {
			return nil, err
		}
		items = append(items, part...)
	}
	return orderByID(items, uniq), nil
}

// loadBooksByIDs 跑一段 IN 查询（chunk 长度必 ≤ maxIDsPerQuery）。
func (s *Store) loadBooksByIDs(ctx context.Context, chunk []int64) ([]BookSummary, error) {
	where := []string{activeBookCond, "b.id IN (" + placeholders(len(chunk)) + ")"}
	args := make([]any, len(chunk))
	for i, id := range chunk {
		args[i] = id
	}
	return s.loadBooks(ctx, where, args, "b.id ASC", 0, 0)
}

// RandomBookSummaries 随机取 n 本书（先数总数再随机 offset）。
//
// 一次性读整库书目在几万本时是几十 MB 的分配，而随机探索只需要十几本：
// 先在 [0, total-n] 里挑一个起点、再取连续 n 行，一次查询就够。
func (s *Store) RandomBookSummaries(ctx context.Context, n int) ([]BookSummary, error) {
	if n <= 0 {
		return nil, nil
	}
	total, err := s.countActiveBooks(ctx)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	size := min(n, total)
	return s.loadBooks(ctx, []string{activeBookCond}, nil, "b.id ASC", size, rand.IntN(total-size+1))
}

// countActiveBooks 数当前版本的书（推荐库里的召回都以「可见书目」为准）。
func (s *Store) countActiveBooks(ctx context.Context) (int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM books WHERE superseded_by = 0").Scan(&total); err != nil {
		return 0, fmt.Errorf("store: count active books: %w", err)
	}
	return total, nil
}

// dedupeIDs 去重并丢掉非正 id；空结果表示「没有要查的对象」。
func dedupeIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// idChunks 把 id 列表切成每段不超过 maxIDsPerQuery 的切片（调用方已去重）。
func idChunks(ids []int64) [][]int64 {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]int64, 0, (len(ids)+maxIDsPerQuery-1)/maxIDsPerQuery)
	for start := 0; start < len(ids); start += maxIDsPerQuery {
		chunks = append(chunks, ids[start:min(start+maxIDsPerQuery, len(ids))])
	}
	return chunks
}

// orderByID 按给定 id 顺序重排查询结果（缺失的 id 直接跳过）。
func orderByID(items []BookSummary, order []int64) []BookSummary {
	byID := make(map[int64]BookSummary, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	out := make([]BookSummary, 0, len(items))
	for _, id := range order {
		if bs, ok := byID[id]; ok {
			out = append(out, bs)
		}
	}
	return out
}
