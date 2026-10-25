package store

import (
	"context"
	"database/sql"
	"fmt"
)

// 评分与批量元数据（P3 管理侧补全）。
//
// rating 直接落在 books 上而不是独立表：单用户、单值，独立表只会多一次 JOIN；
// 0 是「未评」的哨兵值（与 read_status 的 '' 同一套取舍），因此不做 NULL 语义。

// maxBookRating 是评分上限（0 = 未评）。
const maxBookRating = 5

// maxBatchBooks 是一次批量 PATCH 的书籍上限，与契约的 ids ≤ 200 一致。
const maxBatchBooks = 200

// SetBookRating 设置一本书的评分（0–5，0 = 清零）；越界 ErrInvalidArgument，
// 书不存在 ErrNotFound。
func (s *Store) SetBookRating(ctx context.Context, id int64, rating int) error {
	if id <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
	}
	if rating < 0 || rating > maxBookRating {
		return fmt.Errorf("%w: rating %d 必须在 0~%d 之间", ErrInvalidArgument, rating, maxBookRating)
	}
	return s.inTx(ctx, "set book rating", func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"UPDATE books SET rating = ?, updated_at = ? WHERE id = ?",
			rating, formatTime(nowUTC()), id)
		if err != nil {
			return fmt.Errorf("store: set book rating %d: %w", id, err)
		}
		return requireOneRow(res, id)
	})
}

// PatchBooksBatch 在单事务内对多本书应用同一 patch。
//
// 逐本套 SAVEPOINT：某本失败只回滚它自己并记进 failed，其余书照常写入。
// 批量编辑的典型场景是「勾了 30 本、其中 1 本刚被别处删掉」，那不该整批失败。
// 返回的 updated 保持入参顺序；failed 的 key 是书 id，value 是该本失败的原因。
func (s *Store) PatchBooksBatch(ctx context.Context, ids []int64, p BookMetaPatch) ([]int64, map[int64]error, error) {
	bookIDs, err := normalizeBatchIDs(ids)
	if err != nil {
		return nil, nil, err
	}
	if p.IsEmpty() {
		return nil, nil, fmt.Errorf("%w: patch 不能为空", ErrInvalidArgument)
	}
	// 先验一遍标量：非法 patch 是请求错误（api 映射 400），
	// 不该退化成「每本书各失败一次」。
	if _, _, err := buildColumnPatchSets(p); err != nil {
		return nil, nil, err
	}
	updated := make([]int64, 0, len(bookIDs))
	failed := make(map[int64]error)
	if err := s.inTx(ctx, "patch books batch", func(tx *sql.Tx) error {
		for _, id := range bookIDs {
			if err := inSavepoint(ctx, tx, fmt.Sprintf("patch_book_%d", id), func() error {
				return updateBookMetaInTx(ctx, tx, id, p)
			}); err != nil {
				failed[id] = err
				continue
			}
			updated = append(updated, id)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return updated, failed, nil
}

// normalizeBatchIDs 校验批量书 id：非空、不超上限、按入参顺序去重。
func normalizeBatchIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: 至少要指定一本书", ErrInvalidArgument)
	}
	if len(ids) > maxBatchBooks {
		return nil, fmt.Errorf("%w: 一次最多 %d 本", ErrInvalidArgument, maxBatchBooks)
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// inSavepoint 在事务内执行一段可独立回滚的操作：fn 失败时 ROLLBACK TO 保存点，
// 该段已写入的行随之消失，外层事务与别的书的写入不受影响。
func inSavepoint(ctx context.Context, tx *sql.Tx, name string, fn func() error) error {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return fmt.Errorf("store: savepoint %s: %w", name, err)
	}
	if err := fn(); err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rbErr != nil {
			return fmt.Errorf("%w（回滚保存点 %s 失败：%v）", err, name, rbErr)
		}
		_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
		return err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return fmt.Errorf("store: release savepoint %s: %w", name, err)
	}
	return nil
}
