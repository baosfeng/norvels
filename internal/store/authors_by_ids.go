package store

import (
	"context"
	"fmt"
)

// 按 id 批量取作者名（看板与推荐理由的展示层用）。
//
// 为什么单独一个方法而不是让调用方 ListAuthors 再筛：作者表可能几千行，
// 按名字前缀/分页都拿不到「我手上这 10 个 id」的精确结果；一次 IN 是最小代价。

// AuthorNamesByIDs 按 id 批量取作者名（去重、跳过非正 id）。
//
// 缺失的 id 不会出现在返回的 map 里（作者已被删除 / id 本就不存在）：
// 调用方据此决定回退文案，而不是拿到一个空字符串却不知道是「查不到」还是「作者名就是空」。
// 入参超过 maxIDsPerQuery 时分段查询再合并（不报错：分段对调用方透明）。
func (s *Store) AuthorNamesByIDs(ctx context.Context, ids []int64) (map[int64]string, error) {
	uniq := dedupeIDs(ids)
	if len(uniq) == 0 {
		return nil, nil
	}
	out := make(map[int64]string, len(uniq))
	for _, chunk := range idChunks(uniq) {
		if err := s.authorNamesByIDsChunk(ctx, chunk, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// authorNamesByIDsChunk 查一段 id 并把结果并进 out（chunk 长度必 ≤ maxIDsPerQuery）。
func (s *Store) authorNamesByIDsChunk(ctx context.Context, chunk []int64, out map[int64]string) error {
	args := make([]any, len(chunk))
	for i, id := range chunk {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name FROM authors WHERE id IN ("+placeholders(len(chunk))+")", args...)
	if err != nil {
		return fmt.Errorf("store: author names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("store: scan author name: %w", err)
		}
		out[id] = name
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: author names rows: %w", err)
	}
	return nil
}
