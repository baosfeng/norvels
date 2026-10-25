package store

import (
	"context"
	"database/sql"
	"fmt"
)

// books 侧文本量的增量维护。
//
// 与章节正文分开：books 只有千级行，这里的「重算差值」成本可忽略，
// 但口径必须与全量重算（bookTextBytesSQL）严格一致，否则 /api/stats 的
// 体积构成会随导入/覆盖而漂移。

// bumpBookTextBytes 重算单本书的文本字节并把差值并入 book_text_bytes。
//
// books 表只有千级行，按 book_id 取一行是主键查询，代价可忽略；
// 用「重算差值」而不是「猜增量」，因此覆盖写入/标题变更都不会漏。
func bumpBookTextBytes(ctx context.Context, tx *sql.Tx, bookID int64) (int64, error) {
	var current int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(length(title) + length(intro) + length(source_path) + length(source_name) + "+
			"length(encoding) + length(status), 0) FROM books WHERE id = ?", bookID).Scan(&current); err != nil {
		return 0, fmt.Errorf("store: book text bytes: %w", err)
	}
	if err := bumpMetricKey(ctx, tx, metricBookTextBytes, current); err != nil {
		return 0, err
	}
	return current, nil
}

// subtractBookTextBytes 在删除/覆盖前把这本书的文本字节从 book_text_bytes 里减掉。
func subtractBookTextBytes(ctx context.Context, tx *sql.Tx, bookID int64) error {
	var current int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(length(title) + length(intro) + length(source_path) + length(source_name) + "+
			"length(encoding) + length(status), 0) FROM books WHERE id = ?", bookID).Scan(&current); err != nil {
		return fmt.Errorf("store: book text bytes: %w", err)
	}
	return bumpMetricKey(ctx, tx, metricBookTextBytes, -current)
}
