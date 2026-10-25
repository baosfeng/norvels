package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// 卷管理（管理页 C2）：新建 / 改名 / 删除 / 重排，以及章节跨卷移动。
//
// 不变量（与 docs/specs/2026-10-25-book-management-m3.md 一致）：
//   - 同书卷名唯一（去首尾空白后比较），重名返回 ErrVolumeNameTaken（api 映射 409）；
//     由事务内查重与 0013 的表达式唯一索引 uq_volumes_book_title 共同保证——
//     前者给出正常路径的错误，后者兜住「查重与插入之间」的并发竞态；
//   - 卷 idx 在本书内始终是 0..n-1 连续，books.volume_count 与卷行数严格同源；
//   - 移动 / 删除卷只改 chapters.volume_id，不动 chapter.idx（全书序号）、正文与 word_count。
//
// 五个写入口都是单事务：校验与写入在同一事务内，失败整体回滚。

// maxVolumeTitleRunes 与章节标题同口径（前端输入框上限一致）。
const maxVolumeTitleRunes = 200

// volumeIdxShift 是重排卷序时的一次性偏移量：volumes 上有 UNIQUE(book_id, idx)，
// 直接从 0..n-1 逐行改写会在中间态撞唯一约束（把 idx=2 改成 1 时 1 还被占着）。
// 先整体加上偏移量腾空目标区间，再写回 0..n-1；存量 idx 非负，偏移后不会与目标值重叠。
const volumeIdxShift = 1_000_000

// CreateVolume 在本书末尾追加一卷（idx = 当前卷数），成功返回新卷节点。
func (s *Store) CreateVolume(ctx context.Context, bookID int64, title string) (*VolumeNode, error) {
	if bookID <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	clean, err := cleanVolumeTitle(title)
	if err != nil {
		return nil, err
	}
	var out VolumeNode
	err = s.inTx(ctx, "create volume", func(tx *sql.Tx) error {
		return createVolumeInTx(ctx, tx, bookID, clean, &out)
	})
	if err != nil {
		return nil, s.explainVolumeWriteFailure(ctx, bookID, clean, err)
	}
	return &out, nil
}

// createVolumeInTx 是建卷的事务体：查重 → 取下一个 idx → 插入 → 重算 volume_count。
func createVolumeInTx(ctx context.Context, tx *sql.Tx, bookID int64, title string, out *VolumeNode) error {
	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return err
	}
	if err := ensureVolumeNameFree(ctx, tx, bookID, title, 0); err != nil {
		return err
	}
	var idx int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(idx) + 1, 0) FROM volumes WHERE book_id = ?", bookID).Scan(&idx); err != nil {
		return fmt.Errorf("store: next volume idx %d: %w", bookID, err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO volumes(book_id, idx, title) VALUES (?, ?, ?)", bookID, idx, title)
	if err != nil {
		if taken := volumeTitleConflict(err, title); taken != nil {
			return taken
		}
		return fmt.Errorf("store: insert volume %d: %w", bookID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("store: volume id %d: %w", bookID, err)
	}
	if err := refreshVolumeCount(ctx, tx, bookID); err != nil {
		return err
	}
	*out = VolumeNode{ID: id, Idx: idx, Title: title, Chapters: []ChapterBrief{}}
	return nil
}

// RenameVolume 改卷名；卷不属于本书或不存在返回 ErrNotFound。
func (s *Store) RenameVolume(ctx context.Context, bookID, volumeID int64, title string) (*VolumeNode, error) {
	if bookID <= 0 || volumeID <= 0 {
		return nil, fmt.Errorf("%w: book %d volume %d", ErrInvalidArgument, bookID, volumeID)
	}
	clean, err := cleanVolumeTitle(title)
	if err != nil {
		return nil, err
	}
	var out VolumeNode
	err = s.inTx(ctx, "rename volume", func(tx *sql.Tx) error {
		idx, err := loadVolumeIdx(ctx, tx, bookID, volumeID)
		if err != nil {
			return err
		}
		if err := ensureVolumeNameFree(ctx, tx, bookID, clean, volumeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE volumes SET title = ? WHERE id = ? AND book_id = ?", clean, volumeID, bookID); err != nil {
			if taken := volumeTitleConflict(err, clean); taken != nil {
				return taken
			}
			return fmt.Errorf("store: rename volume %d: %w", volumeID, err)
		}
		if err := touchBookUpdatedAt(ctx, tx, bookID); err != nil {
			return err
		}
		out = VolumeNode{ID: volumeID, Idx: idx, Title: clean, Chapters: []ChapterBrief{}}
		return nil
	})
	if err != nil {
		return nil, s.explainVolumeWriteFailure(ctx, bookID, clean, err)
	}
	return &out, nil
}

// DeleteVolume 删一卷：卷内章节落回未分卷（volume_id=0），剩余卷 idx 重排，volume_count 重算。
func (s *Store) DeleteVolume(ctx context.Context, bookID, volumeID int64) error {
	if bookID <= 0 || volumeID <= 0 {
		return fmt.Errorf("%w: book %d volume %d", ErrInvalidArgument, bookID, volumeID)
	}
	return s.inTx(ctx, "delete volume", func(tx *sql.Tx) error {
		if _, err := loadVolumeIdx(ctx, tx, bookID, volumeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE chapters SET volume_id = 0 WHERE book_id = ? AND volume_id = ?", bookID, volumeID); err != nil {
			return fmt.Errorf("store: unassign chapters of volume %d: %w", volumeID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM volumes WHERE id = ? AND book_id = ?", volumeID, bookID); err != nil {
			return fmt.Errorf("store: delete volume %d: %w", volumeID, err)
		}
		if err := renumberVolumeIdx(ctx, tx, bookID); err != nil {
			return err
		}
		return refreshVolumeCount(ctx, tx, bookID)
	})
}

// ReorderVolumes 按给定顺序重排本书全部卷（ids 必须恰好是全部卷 id 的一个排列）。
//
// 排列不完整 / 含非本书卷 / 有重复都是 ErrInvalidArgument（400）：这是调用方参数错，
// 不是「这条卷已经不存在」（那个场景只有单个 id 的接口才有）。
func (s *Store) ReorderVolumes(ctx context.Context, bookID int64, ids []int64) error {
	if bookID <= 0 {
		return fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if err := checkVolumeIDs(ids); err != nil {
		return err
	}
	return s.inTx(ctx, "reorder volumes", func(tx *sql.Tx) error {
		if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
			return err
		}
		if err := ensureVolumeOrder(ctx, tx, bookID, ids); err != nil {
			return err
		}
		if err := writeVolumeOrder(ctx, tx, bookID, ids); err != nil {
			return err
		}
		return refreshVolumeCount(ctx, tx, bookID)
	})
}

// MoveChaptersToVolume 把本书若干章移到指定卷；volumeID=0 表示移到未分卷。
//
// 返回实际移动的章数（= 校验通过的 chapterIds 个数）。chapter.idx / 正文 / word_count
// 一律不动：全书章节序号与阅读进度都按 chapter.idx / id 指过来，移动只是换卷归属。
func (s *Store) MoveChaptersToVolume(ctx context.Context, bookID int64, chapterIDs []int64, volumeID int64) (int, error) {
	if bookID <= 0 {
		return 0, fmt.Errorf("%w: book id %d", ErrInvalidArgument, bookID)
	}
	if volumeID < 0 {
		return 0, fmt.Errorf("%w: volume id %d", ErrInvalidArgument, volumeID)
	}
	ids, err := normalizeChapterIDs(chapterIDs) // 空 / 重复 / 非正整数都是 400
	if err != nil {
		return 0, err
	}
	moved := 0
	err = s.inTx(ctx, "move chapters", func(tx *sql.Tx) error {
		n, txErr := moveChaptersInTx(ctx, tx, bookID, ids, volumeID)
		moved = n
		return txErr
	})
	if err != nil {
		return 0, err
	}
	return moved, nil
}

// moveChaptersInTx 是移动章节的事务体：目标卷存在 → 章节全部属于本书 → 只改 volume_id。
func moveChaptersInTx(ctx context.Context, tx *sql.Tx, bookID int64, ids []int64, volumeID int64) (int, error) {
	if err := ensureBookExistsInTx(ctx, tx, bookID); err != nil {
		return 0, err
	}
	if volumeID > 0 {
		if _, err := loadVolumeIdx(ctx, tx, bookID, volumeID); err != nil {
			return 0, err
		}
	}
	if err := ensureChaptersOfBook(ctx, tx, bookID, ids); err != nil {
		return 0, err
	}
	if err := setChapterVolumes(ctx, tx, bookID, ids, volumeID); err != nil {
		return 0, err
	}
	if err := touchBookUpdatedAt(ctx, tx, bookID); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// cleanVolumeTitle 归一化并校验卷名：去首尾空白、非空、≤200 字。
func cleanVolumeTitle(title string) (string, error) {
	clean := strings.TrimSpace(title)
	if clean == "" {
		return "", fmt.Errorf("%w: 卷名不能为空", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(clean) > maxVolumeTitleRunes {
		return "", fmt.Errorf("%w: 卷名超过 %d 字", ErrInvalidArgument, maxVolumeTitleRunes)
	}
	return clean, nil
}

// ensureVolumeNameFree 检查同书卷名是否已被占用（excludeID>0 时排除自己）。
//
// 比较口径是 TRIM 后的标题：库里可能存着导入时带空白的历史卷名，而用户看到的
// 是两个一模一样的名字——按用户可见形式判重，才与「这本书已经有同名卷了」这句话一致。
func ensureVolumeNameFree(ctx context.Context, tx *sql.Tx, bookID int64, title string, excludeID int64) error {
	query := "SELECT COUNT(*) FROM volumes WHERE book_id = ? AND TRIM(title) = ?"
	args := []any{bookID, title}
	if excludeID > 0 {
		query += " AND id <> ?"
		args = append(args, excludeID)
	}
	var n int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return fmt.Errorf("store: check volume name %d: %w", bookID, err)
	}
	if n > 0 {
		return fmt.Errorf("%w: %s", ErrVolumeNameTaken, title)
	}
	return nil
}

// loadVolumeIdx 读卷在本书内的序号；卷不存在或不属于本书返回 ErrNotFound。
func loadVolumeIdx(ctx context.Context, tx *sql.Tx, bookID, volumeID int64) (int, error) {
	var idx int
	err := tx.QueryRowContext(ctx,
		"SELECT idx FROM volumes WHERE id = ? AND book_id = ?", volumeID, bookID).Scan(&idx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: volume %d of book %d", ErrNotFound, volumeID, bookID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: load volume %d: %w", volumeID, err)
	}
	return idx, nil
}

// listVolumeIDs 按当前卷序返回本书全部卷 id。
func listVolumeIDs(ctx context.Context, tx *sql.Tx, bookID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id FROM volumes WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list volumes %d: %w", bookID, err)
	}
	defer rows.Close()
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan volume id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list volume ids %d: %w", bookID, err)
	}
	return ids, nil
}

// checkVolumeIDs 校验重排入参：每个 id 为正、不重复；空数组在「0 卷的书」上合法。
func checkVolumeIDs(ids []int64) error {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("%w: volume id %d", ErrInvalidArgument, id)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: volume id %d 重复", ErrInvalidArgument, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// ensureVolumeOrder 校验 ids 恰好是本书全部卷 id 的一个排列（数量对 + 逐个归属）。
func ensureVolumeOrder(ctx context.Context, tx *sql.Tx, bookID int64, ids []int64) error {
	owned, err := listVolumeIDs(ctx, tx, bookID)
	if err != nil {
		return err
	}
	if len(ids) != len(owned) {
		return fmt.Errorf("%w: 必须给出本书全部 %d 卷的 id（收到 %d 个）",
			ErrInvalidArgument, len(owned), len(ids))
	}
	for _, id := range ids {
		if !slices.Contains(owned, id) {
			return fmt.Errorf("%w: 卷 %d 不属于这本书", ErrInvalidArgument, id)
		}
	}
	return nil
}

// writeVolumeOrder 把 ids 写成 0..n-1 的卷序（两段式，避开 UNIQUE(book_id, idx) 中间态）。
func writeVolumeOrder(ctx context.Context, tx *sql.Tx, bookID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE volumes SET idx = idx + ? WHERE book_id = ?", volumeIdxShift, bookID); err != nil {
		return fmt.Errorf("store: shift volume idx %d: %w", bookID, err)
	}
	stmt, err := tx.PrepareContext(ctx, "UPDATE volumes SET idx = ? WHERE id = ? AND book_id = ?")
	if err != nil {
		return fmt.Errorf("store: reorder volume prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for i, id := range ids {
		if _, err := stmt.ExecContext(ctx, i, id, bookID); err != nil {
			return fmt.Errorf("store: reorder volume %d: %w", id, err)
		}
	}
	return nil
}

// renumberVolumeIdx 把剩余卷的 idx 按当前顺序重排成 0..n-1。
func renumberVolumeIdx(ctx context.Context, tx *sql.Tx, bookID int64) error {
	ids, err := listVolumeIDs(ctx, tx, bookID)
	if err != nil {
		return err
	}
	return writeVolumeOrder(ctx, tx, bookID, ids)
}

// ensureChaptersOfBook 逐章确认属于本书；任一章不属于即 ErrNotFound。
func ensureChaptersOfBook(ctx context.Context, tx *sql.Tx, bookID int64, ids []int64) error {
	for _, id := range ids {
		var one int
		err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM chapters WHERE id = ? AND book_id = ?", id, bookID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: chapter %d of book %d", ErrNotFound, id, bookID)
		}
		if err != nil {
			return fmt.Errorf("store: check chapter %d: %w", id, err)
		}
	}
	return nil
}

// setChapterVolumes 只改 volume_id：idx / title / 正文 / word_count 都不动。
func setChapterVolumes(ctx context.Context, tx *sql.Tx, bookID int64, ids []int64, volumeID int64) error {
	stmt, err := tx.PrepareContext(ctx, "UPDATE chapters SET volume_id = ? WHERE id = ? AND book_id = ?")
	if err != nil {
		return fmt.Errorf("store: move chapter prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, volumeID, id, bookID); err != nil {
			return fmt.Errorf("store: move chapter %d: %w", id, err)
		}
	}
	return nil
}

// refreshVolumeCount 重算 books.volume_count 并刷新 updated_at（卷结构每次变动都调）。
func refreshVolumeCount(ctx context.Context, tx *sql.Tx, bookID int64) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE books SET volume_count = (SELECT COUNT(*) FROM volumes WHERE book_id = books.id), "+
			"updated_at = ? WHERE id = ?", formatTime(nowUTC()), bookID)
	if err != nil {
		return fmt.Errorf("store: refresh volume count %d: %w", bookID, err)
	}
	return requireOneRow(res, bookID)
}
