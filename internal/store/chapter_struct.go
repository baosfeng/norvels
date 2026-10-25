package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// 章节结构操作的公共底座：批量删除（chapter_delete_many.go）、合并（chapter_merge.go）、
// 重编号（chapter_renumber.go）共用同一套入参校验、序号重排与块回收逻辑。
//
// 抽出来是为了让三条路径守同一个口径：校验各写一份，就会出现「删除拒绝重复 id、
// 合并却放过」这类漏口，而重复 id 会让同一章在计数里被减两次，静默写坏
// books.chapter_count——这种漂移不会立刻报错，只会在几周后的 /api/stats 里露头。

// normalizeChapterIDs 校验批量操作的章节 id 列表，返回保序的结果。
//
// fail-closed：空列表、非正数、重复 id 一律拒绝，不做「尽力而为」。
func normalizeChapterIDs(cids []int64) ([]int64, error) {
	if len(cids) == 0 {
		return nil, fmt.Errorf("%w: 至少要指定一章", ErrInvalidArgument)
	}
	seen := make(map[int64]struct{}, len(cids))
	out := make([]int64, 0, len(cids))
	for _, id := range cids {
		if id <= 0 {
			return nil, fmt.Errorf("%w: chapter id %d", ErrInvalidArgument, id)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("%w: chapter id %d 重复", ErrInvalidArgument, id)
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// loadChapterSpans 按给定顺序读出各章的块定位；任一章不属于这本书即返回 ErrNotFound。
//
// 逐章查而不是拼一条 IN：批量操作的规模是「用户勾选的几十章」，单行主键查询的
// 往返成本可忽略，换来的是与单章路径共用 loadChapterSpan 的同一套错误口径。
func loadChapterSpans(ctx context.Context, tx *sql.Tx, bookID int64, cids []int64) ([]chapterSpan, error) {
	spans := make([]chapterSpan, 0, len(cids))
	for _, id := range cids {
		sp, err := loadChapterSpan(ctx, tx, bookID, id)
		if err != nil {
			return nil, err
		}
		spans = append(spans, sp)
	}
	return spans, nil
}

// deleteChapterRows 批量删除章节行（调用方已确认它们属于这本书）。
func deleteChapterRows(ctx context.Context, tx *sql.Tx, bookID int64, cids []int64) error {
	if len(cids) == 0 {
		return nil
	}
	places := strings.TrimSuffix(strings.Repeat("?,", len(cids)), ",")
	args := make([]any, 0, len(cids)+1)
	args = append(args, bookID)
	for _, id := range cids {
		args = append(args, id)
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM chapters WHERE book_id = ? AND id IN ("+places+")", args...); err != nil {
		return fmt.Errorf("store: delete %d chapters of book %d: %w", len(cids), bookID, err)
	}
	return nil
}

// chapterIdxRow 是重排序号时的游标行。
type chapterIdxRow struct {
	id  int64
	idx int
}

// renumberChapterIdx 把这本书的 idx 按目录顺序重排成 0..n-1。
//
// 用「按当前 (idx, id) 排序后整体重编号」而不是「删掉的章之后整体平移」：
// 存量库可能已经有空洞（早期迁移、手工改库），结构性写操作顺手把不变量修回来，
// 比继续叠加增量平移更可靠。chapters 上没有 UNIQUE(book_id, idx)，
// 中间态出现重复 idx 不会撞约束。
func renumberChapterIdx(ctx context.Context, tx *sql.Tx, bookID int64) error {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, idx FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return fmt.Errorf("store: list chapter idx %d: %w", bookID, err)
	}
	refs := make([]chapterIdxRow, 0, 128)
	for rows.Next() {
		var r chapterIdxRow
		if err := rows.Scan(&r.id, &r.idx); err != nil {
			rows.Close() //nolint:errcheck // 读取失败后关闭，错误已返回
			return fmt.Errorf("store: scan chapter idx: %w", err)
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close() //nolint:errcheck // 同上
	if err != nil {
		return fmt.Errorf("store: list chapter idx rows %d: %w", bookID, err)
	}
	// 先读完再改：边读边改同一张表时游标会看到自己写进去的新 idx，顺序就乱了
	// （同事务里 sqlite 的 UPDATE 可见于此前的 SELECT 游标）。
	return writeChapterIdx(ctx, tx, bookID, refs)
}

// writeChapterIdx 把 refs 的 idx 写成 0..n-1（已经就位的行跳过）。
func writeChapterIdx(ctx context.Context, tx *sql.Tx, bookID int64, refs []chapterIdxRow) error {
	stmt, err := tx.PrepareContext(ctx, "UPDATE chapters SET idx = ? WHERE id = ? AND book_id = ?")
	if err != nil {
		return fmt.Errorf("store: renumber prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for i, r := range refs {
		if r.idx == i {
			continue
		}
		if _, err := stmt.ExecContext(ctx, i, r.id, bookID); err != nil {
			return fmt.Errorf("store: renumber chapter %d: %w", r.id, err)
		}
	}
	return nil
}

// reapBlocksOf 回收这批章节涉及的、已无人引用的块，并校正存活块的章节范围。
func reapBlocksOf(ctx context.Context, tx *sql.Tx, spans []chapterSpan) error {
	seen := make(map[int64]struct{}, len(spans))
	for _, sp := range spans {
		if sp.blockID <= 0 {
			continue
		}
		if _, ok := seen[sp.blockID]; ok {
			continue
		}
		seen[sp.blockID] = struct{}{}
		if err := reapOrphanBlock(ctx, tx, sp.blockID); err != nil {
			return err
		}
		if err := refreshBlockIdxRange(ctx, tx, sp.blockID); err != nil {
			return err
		}
	}
	return nil
}
