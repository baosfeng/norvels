package store

import (
	"context"
	"fmt"
)

// maxCategoryLen 是分类名的字符上限：目录名再长也够用，超长一律截断入库。
const maxCategoryLen = 40

// CategoryCount 是一个分类节点及其书数；L2 为空表示该大分类下未细分。
type CategoryCount struct {
	L1    string `json:"l1"`
	L2    string `json:"l2"`
	Count int    `json:"count"`
}

// ListCategories 聚合大分类→小分类的书目数量，供书架筛选器与分类页使用。
//
// 只读 books 的标量列，不 JOIN 任何表：分类在导入时写在书上。
// 排序按 (l1, l2) 字典序而不是书数，前端据此稳定渲染分组。
func (s *Store) ListCategories(ctx context.Context) ([]CategoryCount, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT category_l1, category_l2, COUNT(*) FROM books "+
			"GROUP BY category_l1, category_l2 ORDER BY category_l1, category_l2")
	if err != nil {
		return nil, fmt.Errorf("store: list categories: %w", err)
	}
	defer rows.Close()

	out := make([]CategoryCount, 0, 32)
	for rows.Next() {
		var c CategoryCount
		if err := rows.Scan(&c.L1, &c.L2, &c.Count); err != nil {
			return nil, fmt.Errorf("store: scan category: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list categories rows: %w", err)
	}
	return out, nil
}
