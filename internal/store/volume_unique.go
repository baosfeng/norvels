package store

// 同书卷名唯一索引（0013）在写入口一侧的兜底逻辑：把「撞唯一索引」与「并发写被打断
// 但库里已有同名卷」两种错误都收敛成 ErrVolumeNameTaken（api 映射 409）。
//
// 与 volume_write.go 分开：那个文件已贴近 400 行门禁，而这段兜底只服务索引，
// 单独成文更好定位（迁移侧建索引见 migrate_steps.go 的 dedupeVolumeTitlesStep）。

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// volumeTitleConflict 把「撞 0013 唯一索引」的写错误翻成 ErrVolumeNameTaken（409）。
//
// 正常路径由 ensureVolumeNameFree 先拦住；走到这里说明查重与写入之间被另一个
// 连接插了同名卷——索引是兜底，错误口径必须和查重一致，前端才不会看到 500。
// 不是唯一冲突时返回 nil，由调用方按原样包装（保留底层错误链）。
func volumeTitleConflict(err error, title string) error {
	if err != nil && strings.Contains(err.Error(), volumeTitleUniqueIndex) {
		return fmt.Errorf("%w: %s", ErrVolumeNameTaken, title)
	}
	return nil
}

// explainVolumeWriteFailure 在写事务失败后，按库里的最新状态解释错误。
//
// 写事务已经是 BEGIN IMMEDIATE（open.go 的 _txlock），正常并发会乖乖等锁；
// 但 busy_timeout 到期、或另一个进程正握着写锁时仍会拿到 SQLITE_BUSY。
// 这种时候如果库里的同名卷已经存在，用户该看到的是 409「换个名字」，
// 而不是 500「稍后重试」。已映射过的错误（重名 / 找不到 / 参数非法）原样返回，
// 不让状态复查改掉它们的优先级。
func (s *Store) explainVolumeWriteFailure(ctx context.Context, bookID int64, title string, err error) error {
	if err == nil || errors.Is(err, ErrVolumeNameTaken) ||
		errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidArgument) {
		return err
	}
	if !s.volumeTitleTakenNow(ctx, bookID, title) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrVolumeNameTaken, title)
}

// volumeTitleTakenNow 用事务外的一次读判断同书是否已有该卷名（含并发写入的那条）。
func (s *Store) volumeTitleTakenNow(ctx context.Context, bookID int64, title string) bool {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM volumes WHERE book_id = ? AND TRIM(title) = ?", bookID, title).Scan(&n)
	return err == nil && n > 0
}
