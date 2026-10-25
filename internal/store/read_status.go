package store

import (
	"context"
	"database/sql"
	"fmt"
)

// books.read_status 的取值（0011 起）。
//
// 与 books.status（解析状态）、books.serial_status（连载状态）互不覆写：
// 导入链路只写 status，PATCH 只写 serial_status，阅读状态只由用户或「首次写进度」写。
const (
	ReadStatusUnset   = ""        // 未标记：存量书与从没标过的书
	ReadStatusWant    = "want"    // 想读
	ReadStatusReading = "reading" // 在读
	ReadStatusDone    = "done"    // 已读
)

// readStatuses 是允许落库的取值集合（白名单：非法值一律拒收）。
var readStatuses = map[string]bool{
	ReadStatusUnset:   true,
	ReadStatusWant:    true,
	ReadStatusReading: true,
	ReadStatusDone:    true,
}

// ValidReadStatus 报告值是否在允许集合内。api 层用它做跨边界校验（非法 → 400），
// store 层用它做出写入守卫（非法 → ErrInvalidArgument），口径只有这一份。
func ValidReadStatus(status string) bool { return readStatuses[status] }

// markReadingOnProgressStep 在写进度的同一事务里把「未标记」的书自动置为在读。
//
// 只从 ” 前进到 reading：want（用户明确想读）与 done（用户明确已读）都不覆盖，
// 否则「重读一本已读完的书」会被悄悄改回在读。
func markReadingOnProgressStep(ctx context.Context, tx *sql.Tx, bookID int64) error {
	if _, err := tx.ExecContext(ctx,
		"UPDATE books SET read_status = ? WHERE id = ? AND read_status = ?",
		ReadStatusReading, bookID, ReadStatusUnset); err != nil {
		return fmt.Errorf("store: mark book %d reading: %w", bookID, err)
	}
	return nil
}
