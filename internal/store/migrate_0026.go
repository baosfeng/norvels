package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// 0026 的全部内容：库级总量（书数 / 书目字数 / 已算质量分书数）的预计算结构与维护触发器。
//
// 迁移只建结构、不做事回填（生产升级期间不能停监听）：表由 0026_book_totals.sql 建好，
// 本步骤补两件 SQL 文件做不到的事：
//  1. 探测 books 是否具备触发器要引用四列（老库 v1 只有 title 等少数列）——
//     缺列就整步跳过，读侧因为状态行缺失而走实时 SQL（慢但正确）；
//  2. 维护触发器——计数正确性靠 SQL 触发器在所有写路径上自动维持，
//     与 0025 同一套理由（见 migrate_0025.go 文件头）。
//
// 回填由启动后的后台任务做（books 一遍扫描），完成前读取侧回退到实时 SQL（数字仍正确）。
// 幂等：触发器全部 IF NOT EXISTS，状态行只在缺失时插入。

func createBookTotalsStep(ctx context.Context, tx *sql.Tx) error {
	ok, err := booksReadyForTotals(ctx, tx)
	if err != nil || !ok {
		return err
	}
	for _, ddl := range bookTotalsTriggerDDL() {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("store: create book totals trigger: %w", err)
		}
	}
	if err := initBookTotalsState(ctx, tx); err != nil {
		return err
	}
	log.Printf("迁移 0026：已建库级总量表与维护触发器（历史值由后台任务回填，不阻塞监听）")
	return nil
}

// booksReadyForTotals 判断 books 是否具备触发器要引用的四列。
//
// 缺任意一列就跳过：老库（v1）的 books 没有 superseded_by / quality_chapters / quality_at，
// 建了触发器只会在写入时报 no such column。读取侧靠状态行缺失判「不可用」走实时 SQL。
func booksReadyForTotals(ctx context.Context, tx *sql.Tx) (bool, error) {
	for _, col := range []string{"superseded_by", "word_count", "quality_chapters", "quality_at"} {
		has, err := columnExistsInTx(ctx, tx, "books", col)
		if err != nil || !has {
			return false, err
		}
	}
	return true, nil
}

// initBookTotalsState 初始化回填状态行（只在缺失时插入）。
//
// 空库直接置 complete=1：之后所有写入都由触发器维持，不存在「历史待补」。
func initBookTotalsState(ctx context.Context, tx *sql.Tx) error {
	var books int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&books); err != nil {
		return fmt.Errorf("store: count books for totals state: %w", err)
	}
	complete := 0
	if books == 0 {
		complete = 1
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO book_totals_state(id, complete, updated_at) VALUES (1, ?, ?)",
		complete, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: init book_totals_state: %w", err)
	}
	return nil
}

// totalsKindCurrent 是「当前版本书数」的 kind（books.superseded_by = 0）。
//
// 与「全量书数」（books_all）分开存：健康报告的书数与书架同口径（只数当前版本），
// 质量分覆盖率的口径是「books 表有多少行」（含已被替换的旧版）。
// 两个数字都由 books 一遍扫描得出，各存一份比在读取侧拿一个去凑另一个安全。
const totalsKindCurrent = "books_current"

// totalsInsertDelta 生成 INSERT 触发器里四个 kind 的增量表达式。
//
// 口径与实时 SQL 逐字一致（见 book_totals.go 的 liveBookTotalsSQL）：
//   - books_all       = COUNT(*) FROM books
//   - books_current   = COUNT(*) FROM books WHERE superseded_by = 0
//   - word_sum        = SUM(word_count) FROM books（含已被替换的旧版）
//   - quality_computed= 质量分「算过」的书数（章数 > 0 且时间戳非空，与 BookQuality.Computed 同口径）
func totalsInsertDelta(row string) string {
	return "UPDATE book_totals SET n = n + (CASE kind" +
		" WHEN 'books_all' THEN 1" +
		" WHEN '" + totalsKindCurrent + "' THEN " + currentHit(row) +
		" WHEN 'word_sum' THEN " + row + ".word_count" +
		" WHEN 'quality_computed' THEN " + qualityHit(row) +
		" ELSE 0 END)"
}

// totalsUpdateDelta 生成 UPDATE 触发器里的增量表达式（NEW − OLD，逐项）。
func totalsUpdateDelta() string {
	return "UPDATE book_totals SET n = n + (CASE kind" +
		" WHEN 'books_all' THEN 0" +
		" WHEN '" + totalsKindCurrent + "' THEN (" + currentHit("NEW") + ") - (" + currentHit("OLD") + ")" +
		" WHEN 'word_sum' THEN NEW.word_count - OLD.word_count" +
		" WHEN 'quality_computed' THEN (" + qualityHit("NEW") + ") - (" + qualityHit("OLD") + ")" +
		" ELSE 0 END)"
}

// currentHit 是「这一行算不算当前版本」的 SQL 表达式。
func currentHit(row string) string {
	return "CASE WHEN " + row + ".superseded_by = 0 THEN 1 ELSE 0 END"
}

// qualityHit 是「这一本书算不算已算质量分」的 SQL 表达式。
func qualityHit(row string) string {
	return "CASE WHEN " + row + ".quality_chapters > 0 AND " + row + ".quality_at <> '' " +
		"THEN 1 ELSE 0 END"
}

// bookTotalsTriggerDDL 生成全部维护触发器（全部 IF NOT EXISTS，重跑幂等）。
//
// 三个写入面：books 的 INSERT / DELETE / UPDATE OF 四列。UPDATE 用 OF 列表而不是
// 无条件触发：改评分、改阅读状态这些高频元数据更新与总量无关，白跑四条行更新没有意义；
// 而四列只要有一列进了 SET 列表就会触发，覆盖率由列名保证，与走哪条写路径无关。
func bookTotalsTriggerDDL() []string {
	return []string{
		"CREATE TRIGGER IF NOT EXISTS books_totals_ai AFTER INSERT ON books BEGIN " +
			totalsInsertDelta("NEW") + "; END",
		"CREATE TRIGGER IF NOT EXISTS books_totals_ad AFTER DELETE ON books BEGIN " +
			"UPDATE book_totals SET n = n - (CASE kind" +
			" WHEN 'books_all' THEN 1" +
			" WHEN '" + totalsKindCurrent + "' THEN " + currentHit("OLD") +
			" WHEN 'word_sum' THEN OLD.word_count" +
			" WHEN 'quality_computed' THEN " + qualityHit("OLD") +
			" ELSE 0 END); END",
		"CREATE TRIGGER IF NOT EXISTS books_totals_au AFTER UPDATE OF " +
			"superseded_by, word_count, quality_chapters, quality_at ON books BEGIN " +
			totalsUpdateDelta() + "; END",
	}
}
