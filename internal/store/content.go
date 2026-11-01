package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// 本文件是「正文独立库」的库级底座：文件推导、表结构、以及**所有写路径**共用的
// 跨库写入编排（先正文库、后主库、失败补偿）。
//
// 为什么不依赖跨库事务：SQLite 的原子提交只覆盖单个数据库文件，WAL 模式下
// ATTACH 两个库的跨库事务也不保证原子（进程崩溃会把它们提交成一半）。
// 因此这里不做「一个事务写两个库」，而是固定顺序 + 事后补偿（设计见
// docs/specs/2026-11-01-content-db-and-dedupe-design.md §2）。

// EnvContentDB 覆盖正文库路径（缺省按主库路径推导同目录兄弟文件）。
//
// 与其它 NORVELS_ 环境变量同一套口径，但解析发生在 store.Open（配置包不参与
// 路径推导：正文库必须与主库同目录同寿命，否则备份/迁移会漏一个文件）。
const EnvContentDB = "NORVELS_CONTENT_DB"

// contentSchemaDDL 是正文库的全部结构，与主库 0007 时代的 chapter_blocks 逐列一致
// （id / book_id / seq / first_idx / last_idx / raw_bytes / packed + 两个索引），
// 唯一区别是**没有外键**——books 在另一个文件里，跨库外键不存在。
//
// 结构只在这里声明一次：0018 迁移建它，新建的库也建它，没有第二份口径。
var contentSchemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS chapter_blocks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id    INTEGER NOT NULL,
    seq        INTEGER NOT NULL,
    first_idx  INTEGER NOT NULL,
    last_idx   INTEGER NOT NULL,
    raw_bytes  INTEGER NOT NULL,
    packed     BLOB    NOT NULL,
    UNIQUE(book_id, seq)
)`,
	"CREATE INDEX IF NOT EXISTS idx_chapter_blocks_range ON chapter_blocks(book_id, first_idx, last_idx)",
	"CREATE INDEX IF NOT EXISTS idx_chapter_blocks_book  ON chapter_blocks(book_id, seq)",
}

// ContentDBPath 由主库路径推导正文库路径（缺省同目录兄弟文件，NORVELS_CONTENT_DB 覆盖）。
//
// 导出给离线工具用（cmd/codebench 等）：**这是推导规则在本仓库里的唯一实现**，
// 新工具不要再自己拼一遍路径——拼错一个字母就是「读到的库没有块表」。
func ContentDBPath(mainPath string) string { return contentDBPath(mainPath) }

// contentDBPath 由主库路径推导正文库路径。
//
// 默认是同目录兄弟文件：norvels.db → norvels-content.db（保留扩展名，便于
// 「看到 db 就知道是一伙的」）。NORVELS_CONTENT_DB 非空时整路径覆盖。
func contentDBPath(mainPath string) string {
	if v := strings.TrimSpace(os.Getenv(EnvContentDB)); v != "" {
		return v
	}
	dir, base := filepath.Split(mainPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem, ext = base, ""
	}
	return filepath.Join(dir, stem+"-content"+ext)
}

// ensureContentSchema 幂等建立正文库结构（0018 迁移与新建库都走这一处）。
func (s *Store) ensureContentSchema(ctx context.Context) error {
	for _, ddl := range contentSchemaDDL {
		if _, err := s.cdb.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("store: content schema: %w", err)
		}
	}
	return nil
}

// ---------- 跨库写入编排 ----------

// contentTx 是正文库的一次写事务，并记下本次写入 / 待回收的块 id。
type contentTx struct {
	tx *sql.Tx
	// written 是本次写入的块 id：主库提交失败时按 id 精确删除（补偿）。
	written []int64
	// reaped 是本次操作解引用、但**要等主库提交后**才真正删除的块 id。
	//
	// 为什么延后：正文库事务先提交主库，若此刻就删旧块而主库提交失败，
	// 旧章节行仍在、块却没了——那是悬空引用（正文读不出来），比孤儿块严重得多。
	// 延后删除把最坏情况降级成「留孤儿块」，由 ReconcileContent 清掉。
	reaped []int64
}

// writeTx 是一次跨两库的写入：主库事务 + 正文库事务。
type writeTx struct {
	main    *sql.Tx
	content *contentTx
}

// writeThrough 是**所有正文写路径的唯一入口**：开两个事务 → 跑 apply →
// 先提交正文库、再提交主库 → 主库失败时按块 id 删掉本次新写的块。
//
// 顺序固定为「正文先提交」，因为反向（主库先提交）失败时留下的是悬空引用——
// 章节指着不存在的块，读者直接读不出正文；而正向上限只是孤儿块（体积虚高，
// 可被对账清掉）。两害相权，孤儿块可接受，读不出正文不可接受。
func (s *Store) writeThrough(ctx context.Context, what string, apply func(w *writeTx) error) error {
	ct, err := s.cdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %s begin content: %w", what, err)
	}
	defer ct.Rollback() //nolint:errcheck // 提交后回滚是空操作
	mt, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: %s begin: %w", what, err)
	}
	defer mt.Rollback() //nolint:errcheck // 提交后回滚是空操作

	w := &writeTx{main: mt, content: &contentTx{tx: ct}}
	if err := apply(w); err != nil {
		return err
	}
	if err := ct.Commit(); err != nil {
		return fmt.Errorf("store: %s commit content: %w", what, err)
	}
	if err := mt.Commit(); err != nil {
		s.deleteBlocks(ctx, w.content.written)
		return fmt.Errorf("store: %s commit: %w", what, err)
	}
	s.reapBlocks(ctx, w.content.reaped)
	return nil
}

// scheduleReap 记下「本次操作已解引用的块」，真正删除发生在主库提交之后。
func (c *contentTx) scheduleReap(blockID int64) {
	if blockID > 0 {
		c.reaped = append(c.reaped, blockID)
	}
}

// reapBlocks 在主库提交后回收已被解引用的块（失败只记日志：孤儿块由对账清掉）。
func (s *Store) reapBlocks(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	if err := s.deleteBlocks(ctx, ids); err != nil {
		log.Printf("WARN: 回收正文块失败（%d 块，留作孤儿，可跑对账清理）: %v", len(ids), err)
	}
}

// deleteBlocks 按块 id 精确删除正文（补偿与回收共用；自带事务）。
func (s *Store) deleteBlocks(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.cdb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete blocks begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作
	if err := deleteBlockRows(ctx, tx, ids); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteBlockRows 在一个正文库事务里按 id 批量删除块。
func deleteBlockRows(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for start := 0; start < len(ids); start += deleteBatchSize {
		end := start + deleteBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		args := make([]any, 0, len(batch))
		for _, id := range batch {
			args = append(args, id)
		}
		query := "DELETE FROM chapter_blocks WHERE id IN (" + placeholders(len(batch)) + ")"
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("store: delete %d blocks: %w", len(batch), err)
		}
	}
	return nil
}

// purgeBookContent 删除一本书在正文库的全部块（删书的提交后动作、
// 以及主库写失败后的补偿路径都用它）。best-effort：失败只记日志。
func (s *Store) purgeBookContent(ctx context.Context, bookID int64) {
	if bookID <= 0 {
		return
	}
	// 迁移停在 0018 之前的库：块还在主库，由 books 的 ON DELETE CASCADE 一并清掉。
	if ok, err := s.contentTableExists(ctx); err != nil || !ok {
		return
	}
	if _, err := s.cdb.ExecContext(ctx, "DELETE FROM chapter_blocks WHERE book_id = ?", bookID); err != nil {
		log.Printf("WARN: 删除书 %d 的正文块失败（留作孤儿，可跑对账清理）: %v", bookID, err)
	}
}

// placeholders 生成 "?,?,?"（调用方保证 n > 0）。
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// deleteBatchSize 是批量删除的每批行数（SQLite 的变量上限 999 留余量）。
const deleteBatchSize = 500
