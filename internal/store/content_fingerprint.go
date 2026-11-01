package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"norvels/internal/model"
)

// 本文件是正文库 chapter_fingerprints 的**唯一写入口径**：一本书的指纹怎么写、
// 覆盖与追更时怎么清旧行、删书时怎么清指纹，都只在这里出现一次。
//
// 指纹是派生数据：原文在 chapter_blocks 与 chapters 里都在，写坏/写漏只影响判重提示，
// 不影响阅读。所以它不进 writeThrough 的双库编排（那边一条错误就会让整批导入失败），
// 失败按 WARN 处理，下一次导入或重建会自我修正。

// writeBookFingerprintsAfterCommit 是 SaveBook 成功提交后的派生数据收尾：重写本书指纹。
//
// 为什么不在 writeThrough 里：指纹是可重建的派生数据，进那个双库事务等于让一条辅助表
// 决定整批导入的成败；失败只记 WARN（见 saveBookFingerprints）。
// Skipped 的书章节没变，指纹已是当下口径，不重写。
func (s *Store) writeBookFingerprintsAfterCommit(ctx context.Context, res SaveResult, chapters []model.ParsedChapter) {
	if res.BookID > 0 && !res.Skipped {
		s.saveBookFingerprints(ctx, res.BookID, chapters)
	}
}

// saveBookFingerprints 用本书当前的章节重写指纹（同一本书先删后写）。
//
// 为什么整本重写而不是增量维护：覆盖导入与追更都会让章节集合变长或变短，增量维护要
// 先判断「哪一章被删了 / 被改了」，而一章一行的写入成本可以忽略——整本重写是唯一
// 没有漂移风险的做法（旧指纹残留会让追更后的书被自己的旧版本判成重复）。
func (s *Store) saveBookFingerprints(ctx context.Context, bookID int64, chapters []model.ParsedChapter) {
	fps := chapterFingerprints(chapters)
	if bookID <= 0 || len(fps) == 0 {
		return
	}
	if err := s.replaceFingerprints(ctx, bookID, fps); err != nil {
		log.Printf("WARN: 写书 %d 的正文指纹失败（判重会少这本书，重导或重建可修正）: %v", bookID, err)
	}
}

// replaceFingerprints 在一个正文库事务里「删本书旧指纹 + 写新指纹」。
func (s *Store) replaceFingerprints(ctx context.Context, bookID int64, fps []chapterFP) error {
	tx, err := s.cdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: fingerprints begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	if err := deleteFingerprintRows(ctx, tx, bookID); err != nil {
		return err
	}
	if err := insertFingerprintRows(ctx, tx, bookID, fps); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: fingerprints commit book=%d: %w", bookID, err)
	}
	return nil
}

// deleteFingerprintRows 删掉一本书的全部指纹行。
func deleteFingerprintRows(ctx context.Context, tx *sql.Tx, bookID int64) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM chapter_fingerprints WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("store: delete fingerprints book=%d: %w", bookID, err)
	}
	return nil
}

// insertFingerprintRows 逐行写指纹。
//
// INSERT OR REPLACE 而不是 INSERT：同一本书里内容完全相同的两章会撞主键
// (hash, book_id)，收敛成一行即可——判重只问「有没有」，不数它出现了几次。
func insertFingerprintRows(ctx context.Context, tx *sql.Tx, bookID int64, fps []chapterFP) error {
	for _, fp := range fps {
		if _, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO chapter_fingerprints(hash, book_id, bytes) VALUES (?, ?, ?)",
			fp.hash, bookID, fp.bytes); err != nil {
			return fmt.Errorf("store: insert fingerprint book=%d: %w", bookID, err)
		}
	}
	return nil
}

// purgeBookFingerprints 删除一本书的全部指纹（删书的提交后动作）。
//
// best-effort：删书已经提交，指纹残留只会让判重提示指向一本不存在的书
// （bookTitleByID 返回空串，文案走「书名未知」），不值得让删书失败。
func (s *Store) purgeBookFingerprints(ctx context.Context, bookID int64) {
	if bookID <= 0 {
		return
	}
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil || !ready {
		return
	}
	if _, err := s.cdb.ExecContext(ctx,
		"DELETE FROM chapter_fingerprints WHERE book_id = ?", bookID); err != nil {
		log.Printf("WARN: 删除书 %d 的正文指纹失败（留作孤儿，会让判重提示指向已删的书）: %v", bookID, err)
	}
}

// fingerprintTableExists 判断正文库有没有 chapter_fingerprints（0019 之前的库没有）。
func (s *Store) fingerprintTableExists(ctx context.Context) (bool, error) {
	var n int
	err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'chapter_fingerprints'").Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check fingerprint table: %w", err)
	}
	return n > 0, nil
}

// fingerprintRowCount 返回指纹行数（迁移日志与健康诊断用）。
func (s *Store) fingerprintRowCount(ctx context.Context) (int64, error) {
	var n int64
	if err := s.cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM chapter_fingerprints").Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count fingerprints: %w", err)
	}
	return n, nil
}
