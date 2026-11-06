package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"strings"
)

// 随机取一本（P3 §筛选与发现）的存储侧实现。
//
// 为什么不再一律用「数总数 + 随机 page/offset」：offset 落在排序索引上时，SQLite 要
// 逐行走过前 offset 行（每行还要补一次 authors 才能产出结果行），17 万本书目下平均
// 扫掉半张表，生产实测 0.39~0.61 s。无筛选是绝大多数请求，改走「随机 id + 空窗口回退」：
// 用 idx_books_active_id 直接定位到 >= 随机 id 的第一本，代价是一次索引查找。

// randomProbes 是随机 id 落空后的重试上限（落空 = 落在被删书留下的空洞里，
// 或落在 [最后一本的 id, maxID] 之间）。重试仍落空就回退到第一本，
// 避免在稀疏 id 上反复尝试。
const randomProbes = 3

// RandomBook 随机返回一本符合条件的书；没有命中返回 (nil, nil)。
//
// 无筛选走 id 采样；带筛选（作者 / 标签 / 书单 / 分类 / 状态）时命中集通常只有几千本，
// 仍走「数总数 + 随机偏移」，且与书籍列表共用 bookFilter，口径不会分叉。
func (s *Store) RandomBook(ctx context.Context, q BookQuery) (*BookSummary, error) {
	if !q.hasRandomFilter() {
		return s.randomActiveBook(ctx)
	}
	return s.randomFilteredBook(ctx, q)
}

// hasRandomFilter 判断随机请求是否带了缩范围条件（维度与 bookFilter 一一对应）。
func (q BookQuery) hasRandomFilter() bool {
	return strings.TrimSpace(q.Q) != "" || q.AuthorID != 0 || strings.TrimSpace(q.Tag) != "" ||
		q.Collection != 0 || strings.TrimSpace(q.ReadStatus) != "" ||
		strings.TrimSpace(q.SerialStatus) != "" || strings.TrimSpace(q.Series) != "" ||
		q.Rating > 0 || strings.TrimSpace(q.CategoryL1) != ""
}

// randomActiveBook 在「全部未替换的书」里均匀地随机取一本。
func (s *Store) randomActiveBook(ctx context.Context) (*BookSummary, error) {
	minID, maxID, err := s.activeIDRange(ctx)
	if err != nil {
		return nil, err
	}
	if maxID == 0 {
		return nil, nil // 空库
	}
	for i := 0; i < randomProbes; i++ {
		pick := rand.Int64N(maxID-minID+1) + minID
		bs, err := s.firstActiveBookFrom(ctx, pick)
		if err != nil || bs != nil {
			return bs, err
		}
	}
	// 空窗口回退：随机 id 都落在末尾空洞里时，从最小 id 起取第一本。
	return s.firstActiveBookFrom(ctx, minID)
}

// activeIDRange 返回当前版本书目的 id 区间；空库返回 (0, 0)。
//
// MIN 与 MAX 分成两条语句：写在一起（SELECT MIN(id), MAX(id)）时优化器只能整扫一遍
// 覆盖索引（17 万本实测 8.9 ms），分开则两条都是索引两端的一次定位（各 0.3 ms）。
func (s *Store) activeIDRange(ctx context.Context) (int64, int64, error) {
	minID, err := s.activeIDBound(ctx, "MIN")
	if err != nil {
		return 0, 0, err
	}
	maxID, err := s.activeIDBound(ctx, "MAX")
	if err != nil {
		return 0, 0, err
	}
	if !minID.Valid || !maxID.Valid {
		return 0, 0, nil // 空库（聚合返回 NULL）
	}
	return minID.Int64, maxID.Int64, nil
}

// activeIDBound 取 id 的一个端点（fn 只接受 MIN / MAX，是内部常量）。
func (s *Store) activeIDBound(ctx context.Context, fn string) (sql.NullInt64, error) {
	var id sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		"SELECT "+fn+"(id) FROM books WHERE superseded_by = 0").Scan(&id); err != nil {
		return id, fmt.Errorf("store: random book %s(id): %w", fn, err)
	}
	return id, nil
}

// firstActiveBookFrom 取 id >= from 的第一本当前版本的书；没有返回 (nil, nil)。
func (s *Store) firstActiveBookFrom(ctx context.Context, from int64) (*BookSummary, error) {
	where := []string{activeBookCond, "b.id >= ?"}
	items, err := s.loadBooks(ctx, where, []any{from}, "b.id ASC", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

// randomFilteredBook 是带筛选的随机：先数命中数，再按 id 序随机偏移取一行。
//
// 排序用 b.id ASC 而不是列表页的「最近更新」：随机与顺序无关，而 id 序能直接走
// idx_books_active_id（不产生临时 B-TREE）；沿用 updated 序时计划里会多一次
// USE TEMP B-TREE FOR ORDER BY，命中集大时反而比改动前更慢。
// 命中的集合与 bookFilter 完全同一份，均匀性也与按 updated 序取样一致。
func (s *Store) randomFilteredBook(ctx context.Context, q BookQuery) (*BookSummary, error) {
	where, args := bookFilter(ctx, q)
	total, err := s.countBooks(ctx, where, args, strings.TrimSpace(q.Q) != "")
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}
	items, err := s.loadBooks(ctx, where, args, "b.id ASC", 1, rand.IntN(total))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil // 两次查询之间书被删掉了：按「没有」处理，由前端再点一次
	}
	return &items[0], nil
}
