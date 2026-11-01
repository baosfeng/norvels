package store

import (
	"context"
	"fmt"
)

// 按 id 批量取书的标签（推荐埋点的 tag 兴趣加权用）。
//
// 为什么不用 ListBookTags 逐个取：推荐一次埋点批次最多 100 条事件、
// 涉及几十本书，逐本查就是几十次主键往返；一次 IN + JOIN 只查一遍。

// BookTagsByIDs 返回「书 id → 标签名切片（按名字升序）」，没标签的书不出现在 map 里。
//
// 缺失（书不存在 / 没打标签）的 id 直接缺席，而不是给一个空切片：
// 调用方据此区分「这本书没有标签」与「这次没查到」，而不必为一个空值再判断一次。
func (s *Store) BookTagsByIDs(ctx context.Context, ids []int64) (map[int64][]string, error) {
	uniq, err := dedupeIDs(ids, "书")
	if err != nil {
		return nil, err
	}
	if len(uniq) == 0 {
		return nil, nil
	}
	args := make([]any, len(uniq))
	for i, id := range uniq {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT bt.book_id, t.name FROM book_tags bt JOIN tags t ON t.id = bt.tag_id "+
			"WHERE bt.book_id IN ("+placeholders(len(uniq))+") ORDER BY bt.book_id, t.name",
		args...)
	if err != nil {
		return nil, fmt.Errorf("store: book tags by ids: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]string, len(uniq))
	for rows.Next() {
		var (
			bookID int64
			name   string
		)
		if err := rows.Scan(&bookID, &name); err != nil {
			return nil, fmt.Errorf("store: scan book tag: %w", err)
		}
		out[bookID] = append(out[bookID], name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: book tags by ids rows: %w", err)
	}
	return out, nil
}
