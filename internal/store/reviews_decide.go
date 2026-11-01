package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// 本文件是审核记录的状态机与替换语义（设计 §7）：AI 写回、人工决策、回滚。
//
// 所有写路径都是「一个事务 + 条件更新」：导入链路与后台 worker 可能同时写同一条
// 记录，状态迁移以库里的当前值为准（UPDATE ... WHERE id = ? AND status = ?），
// 影响行数为 0 即 ErrReviewState（api 映射 409），不会出现「后写覆盖先写」。

// FinishImportReviewAI 写回一次 AI 裁决结果（ready）或降级结论（needs_manual）。
//
// 条件更新：只有仍是 pending 的记录会被改写。用户已经决策过的记录不允许被后台
// 覆盖——那会让「我点了保留，AI 后到又把状态改回 ready」这种状态错乱出现。
func (s *Store) FinishImportReviewAI(ctx context.Context, id int64, res ReviewAIResult) error {
	status, winner, reasons, err := normalizeAIResult(res)
	if err != nil {
		return err
	}
	raw, err := encodeReasons(reasons)
	if err != nil {
		return err
	}
	n, err := s.execAffected(ctx,
		"UPDATE import_reviews SET status = ?, ai_winner = ?, ai_confidence = ?, ai_reasons = ? "+
			"WHERE id = ? AND status = ?",
		status, winner, clampRatio(res.Confidence), raw, id, ReviewStatusPending)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: 审核记录 %d 不是 pending（可能已被决策）", ErrReviewState, id)
	}
	return nil
}

// DecideImportReview 执行一次人工决策（replace / keep），返回决策后的记录。
//
// replace：旧书标 superseded（指向新书）、新书取消 superseded（成为当前版本）；
// keep：只改状态（新书入库时就已隐藏，不需要再动书）。
func (s *Store) DecideImportReview(ctx context.Context, id int64, action, decidedBy string) (ImportReview, error) {
	if action != ReviewActionReplace && action != ReviewActionKeep {
		return ImportReview{}, fmt.Errorf("%w: 未知决策 %q（只支持 replace / keep）", ErrInvalidArgument, action)
	}
	return s.decideReview(ctx, id, action, decidedBy)
}

// RollbackImportReview 把一次已确认的替换回滚到旧版本。
//
// 只允许从 confirmed_replace 回滚：keep 的旧版本本来就是当前版本，回滚没有语义；
// ready / needs_manual 还没替换过，也没有可回滚的东西。
func (s *Store) RollbackImportReview(ctx context.Context, id int64, decidedBy string) (ImportReview, error) {
	return s.decideReview(ctx, id, reviewActionRollback, decidedBy)
}

// decideReview 是 decide / rollback 的共用事务：读记录 → 校验状态机 → 改书 → 改状态。
func (s *Store) decideReview(ctx context.Context, id int64, action, decidedBy string) (ImportReview, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportReview{}, fmt.Errorf("store: begin review decide: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	rev, err := loadReviewTx(ctx, tx, id)
	if err != nil {
		return ImportReview{}, err
	}
	if err := applyReviewAction(ctx, tx, rev, action); err != nil {
		return ImportReview{}, err
	}
	next, err := updateReviewDecision(ctx, tx, rev, action, decidedBy)
	if err != nil {
		return ImportReview{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImportReview{}, fmt.Errorf("store: commit review decide: %w", err)
	}
	committed = true
	return next, nil
}

// applyReviewAction 校验状态机并改动书侧（books.superseded_by）。
func applyReviewAction(ctx context.Context, tx *sql.Tx, rev ImportReview, action string) error {
	if action == reviewActionRollback {
		return applyRollback(ctx, tx, rev)
	}
	if err := requireDecidable(rev, action); err != nil {
		return err
	}
	if action == ReviewActionKeep {
		return nil // 保留旧版本：新版本入库时就已隐藏，书侧不用动
	}
	// 旧书被新书取代、新书成为当前版本（两次 UPDATE 在同一事务里）。
	if err := setSupersededBy(ctx, tx, rev.OldBookID, rev.NewBookID); err != nil {
		return err
	}
	return setSupersededBy(ctx, tx, rev.NewBookID, 0)
}

// applyRollback 把已替换的一对书换回来：旧书恢复可见，新书重新隐藏。
func applyRollback(ctx context.Context, tx *sql.Tx, rev ImportReview) error {
	if rev.Status != ReviewStatusConfirmedReplace {
		return fmt.Errorf("%w: 审核记录 %d 状态是 %s，只有 confirmed_replace 能回滚",
			ErrReviewState, rev.ID, rev.Status)
	}
	if err := setSupersededBy(ctx, tx, rev.OldBookID, 0); err != nil {
		return err
	}
	return setSupersededBy(ctx, tx, rev.NewBookID, rev.OldBookID)
}

// requireDecidable 校验记录是否还能被人工决策（pending / ready / needs_manual）。
func requireDecidable(rev ImportReview, action string) error {
	switch rev.Status {
	case ReviewStatusPending, ReviewStatusReady, ReviewStatusNeedsManual:
		return nil
	}
	return fmt.Errorf("%w: 审核记录 %d 状态是 %s，不能再 %s",
		ErrReviewState, rev.ID, rev.Status, action)
}

// decisionStatus 把动作映射成目标状态。
func decisionStatus(action string) string {
	switch action {
	case ReviewActionKeep:
		return ReviewStatusConfirmedKeep
	case ReviewActionReplace:
		return ReviewStatusConfirmedReplace
	default:
		return ReviewStatusRolledBack
	}
}

// updateReviewDecision 按动作写状态与决策人，返回决策后的记录。
func updateReviewDecision(ctx context.Context, tx *sql.Tx, rev ImportReview, action, decidedBy string) (ImportReview, error) {
	now := nowUTC()
	by := strings.TrimSpace(decidedBy)
	n, err := execAffectedTx(ctx, tx,
		"UPDATE import_reviews SET status = ?, decided_at = ?, decided_by = ? WHERE id = ? AND status = ?",
		decisionStatus(action), formatTime(now), by, rev.ID, rev.Status)
	if err != nil {
		return ImportReview{}, err
	}
	if n == 0 {
		// 同一记录被并发决策：状态已被别人改掉，按状态机冲突处理。
		return ImportReview{}, fmt.Errorf("%w: 审核记录 %d 状态已变化", ErrReviewState, rev.ID)
	}
	rev.Status = decisionStatus(action)
	rev.DecidedAt = &now
	rev.DecidedBy = by
	return rev, nil
}

// loadReviewTx 在事务内读一条记录（不存在返回 ErrNotFound）。
func loadReviewTx(ctx context.Context, tx *sql.Tx, id int64) (ImportReview, error) {
	row := tx.QueryRowContext(ctx, "SELECT "+reviewColumns+" FROM import_reviews WHERE id = ?", id)
	rev, err := scanReview(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ImportReview{}, fmt.Errorf("%w: import review %d", ErrNotFound, id)
	case err != nil:
		return ImportReview{}, fmt.Errorf("store: get import review %d: %w", id, err)
	}
	return rev, nil
}

// setSupersededBy 把一本书的 superseded_by 改成 target（0 = 当前版本）。
//
// 书不存在返回 ErrNotFound：替换路径上「旧书被删了」必须显式失败，否则会出现
// 「替换成功、但旧版本还在书架上」的半截状态。
func setSupersededBy(ctx context.Context, tx *sql.Tx, bookID, target int64) error {
	n, err := execAffectedTx(ctx, tx, "UPDATE books SET superseded_by = ? WHERE id = ?", target, bookID)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
}

// requireBookTx 确认书存在（不存在返回 ErrNotFound）。
func requireBookTx(ctx context.Context, tx *sql.Tx, bookID int64) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM books WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check book %d: %w", bookID, err)
	}
	if !exists {
		return fmt.Errorf("%w: book %d", ErrNotFound, bookID)
	}
	return nil
}

// execAffectedTx 执行一条写语句并返回影响行数。
func execAffectedTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("store: exec: %w", err)
	}
	return res.RowsAffected()
}

// execAffected 是 execAffectedTx 的非事务版本。
func (s *Store) execAffected(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("store: exec: %w", err)
	}
	return res.RowsAffected()
}
