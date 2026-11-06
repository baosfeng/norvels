package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// 正文库与主库之间没有事务、也没有外键，一致性靠「写入顺序 + 事后补偿」维持
// （见 content.go）。对账是最后一道闸：它把两库之间**不该存在的**关系找出来。
//
// 两类问题及其成因：
//   - 孤儿块：book_id 在主库里不存在。来源是「删书/覆盖的书主库提交成功、正文库
//     清理失败」或进程在两步之间被杀。孤儿块只占体积，不影响阅读。
//   - 悬空引用：chapters.block_id 指向正文库里不存在的块。来源是数据库文件被单独
//     替换/恢复（只恢复了主库）或块被手工删除。这一类是**读不出正文**的故障。

// ContentReconcileReport 是一次正文对账的结果。
type ContentReconcileReport struct {
	OrphanBlocks int `json:"orphanBlocks"` // book_id 不在主库的块数
	OrphanBooks  int `json:"orphanBooks"`  // 上面这些块涉及的书数
	DanglingRefs int `json:"danglingRefs"` // 指向不存在块的章节数
	// RemovedBlocks 是 fix=true 时实际删除的块数（fix=false 时恒为 0）。
	RemovedBlocks int `json:"removedBlocks"`
	// MissingTable 表示正文库里没有 chapter_blocks：0018 之前的库、或正文库文件丢失。
	MissingTable bool      `json:"missingTable"`
	CheckedAt    time.Time `json:"checkedAt"`
}

// ReconcileContent 扫描两个库的引用关系：孤儿块（book_id 不在主库）与悬空引用
// （chapters.block_id 不存在）。fix=true 时删除孤儿块，悬空引用只报告不修
// （缺的正文无法凭空造出来，只能重导或从备份恢复）。
//
// 成本：孤儿侧扫正文库的块表（生产 87k 行，毫秒级）；悬空侧要把 chapters 的
// block_id 全部过一遍（生产 2,469 万行，数十秒级）。因此**不要放在请求路径上**：
// HealthReport 只读快照（见 health_refresh.go），本函数仍保持同步语义给 CLI 用。
//
// 每次跑完都把结果记进快照：这样运维手动跑一次 reconcile-content 之后，
// 驾驶舱立刻看到的就是这份新鲜结果，而不是等下一次后台刷新。
func (s *Store) ReconcileContent(ctx context.Context, fix bool) (ContentReconcileReport, error) {
	rep, err := s.reconcileContent(ctx, fix)
	s.recordContentSnapshot(rep, err)
	return rep, err
}

// reconcileContent 是对账本体（不带快照记账），供上面这层与本包测试直接调用。
func (s *Store) reconcileContent(ctx context.Context, fix bool) (ContentReconcileReport, error) {
	rep := ContentReconcileReport{CheckedAt: nowUTC()}
	exists, err := s.contentTableExists(ctx)
	if err != nil {
		return rep, err
	}
	if !exists {
		rep.MissingTable = true
		return rep, nil
	}
	orphanBooks, orphanBlocks, err := s.findOrphanContent(ctx)
	if err != nil {
		return rep, err
	}
	rep.OrphanBooks, rep.OrphanBlocks = len(orphanBooks), orphanBlocks
	if fix && len(orphanBooks) > 0 {
		removed, err := s.removeOrphanContent(ctx, orphanBooks)
		if err != nil {
			return rep, err
		}
		rep.RemovedBlocks = removed
	}
	dangling, err := s.countDanglingRefs(ctx)
	if err != nil {
		return rep, err
	}
	rep.DanglingRefs = dangling
	return rep, nil
}

// findOrphanContent 返回「在正文库有块、但主库里没有这本书」的 book_id 列表与块数。
//
// 先按 book_id 聚合（生产 87k 块 / 2 万本，一次分组扫描），再逐批到主库确认存在性：
// 两库不能 JOIN，只能把一侧的键集合搬到另一侧比对。
func (s *Store) findOrphanContent(ctx context.Context) ([]int64, int, error) {
	counts, err := s.contentBookBlockCounts(ctx)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	live, err := s.existingBookIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	orphans := make([]int64, 0, len(ids))
	blocks := 0
	for _, id := range ids {
		if _, ok := live[id]; ok {
			continue
		}
		orphans = append(orphans, id)
		blocks += counts[id]
	}
	return orphans, blocks, nil
}

// contentBookBlockCounts 返回正文库里每本书的块数。
func (s *Store) contentBookBlockCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := s.cdb.QueryContext(ctx, "SELECT book_id, COUNT(*) FROM chapter_blocks GROUP BY book_id")
	if err != nil {
		return nil, fmt.Errorf("store: reconcile content books: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	out := make(map[int64]int, 1024)
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: reconcile scan content book: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reconcile content books rows: %w", err)
	}
	return out, nil
}

// existingBookIDs 返回给定 id 里**确实存在于主库**的那些。
func (s *Store) existingBookIDs(ctx context.Context, ids []int64) (map[int64]struct{}, error) {
	live := make(map[int64]struct{}, len(ids))
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
		rows, err := s.db.QueryContext(ctx,
			"SELECT id FROM books WHERE id IN ("+placeholders(len(batch))+")", args...)
		if err != nil {
			return nil, fmt.Errorf("store: reconcile books: %w", err)
		}
		got, err := scanIDs(rows)
		if err != nil {
			return nil, err
		}
		for _, id := range got {
			live[id] = struct{}{}
		}
	}
	return live, nil
}

// removeOrphanContent 删除这些书在正文库的全部块，返回实际删除的行数。
func (s *Store) removeOrphanContent(ctx context.Context, bookIDs []int64) (int, error) {
	tx, err := s.cdb.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: reconcile begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后回滚是空操作

	removed := 0
	for start := 0; start < len(bookIDs); start += deleteBatchSize {
		end := start + deleteBatchSize
		if end > len(bookIDs) {
			end = len(bookIDs)
		}
		batch := bookIDs[start:end]
		args := make([]any, 0, len(batch))
		for _, id := range batch {
			args = append(args, id)
		}
		res, err := tx.ExecContext(ctx,
			"DELETE FROM chapter_blocks WHERE book_id IN ("+placeholders(len(batch))+")", args...)
		if err != nil {
			return 0, fmt.Errorf("store: reconcile delete orphan blocks: %w", err)
		}
		n, _ := res.RowsAffected()
		removed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: reconcile commit: %w", err)
	}
	return removed, nil
}

// countDanglingRefs 统计指向不存在块的章节数。
//
// 先取正文库的块 id 集合（生产 87k 个，几 MB），再按 id 游标分批扫 chapters 的
// block_id：两库不能 JOIN，但「小集合 × 大表」的比对可以只用一次主库扫描。
// 分批（每批 reconcileScanBatch 行）是为了内存有界，总成本仍是全表一遍。
func (s *Store) countDanglingRefs(ctx context.Context) (int, error) {
	blocks, err := s.contentBlockIDSet(ctx)
	if err != nil {
		return 0, err
	}
	dangling := 0
	lastID := int64(0)
	for {
		maxID, bad, err := s.scanChapterRefs(ctx, blocks, lastID)
		if err != nil {
			return 0, err
		}
		dangling += bad
		if maxID == 0 {
			return dangling, nil
		}
		lastID = maxID
	}
}

// contentBlockIDSet 读出正文库全部块 id。
func (s *Store) contentBlockIDSet(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := s.cdb.QueryContext(ctx, "SELECT id FROM chapter_blocks")
	if err != nil {
		return nil, fmt.Errorf("store: reconcile block ids: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// scanChapterRefs 扫一批章节（id > afterID），返回本批最大 id 与悬空条数。
//
// 最大 id 为 0 表示已经扫完；调用方据此结束循环。
func (s *Store) scanChapterRefs(ctx context.Context, blocks map[int64]struct{}, afterID int64) (int64, int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, block_id FROM chapters WHERE block_id > 0 AND id > ? ORDER BY id LIMIT ?",
		afterID, reconcileScanBatch)
	if err != nil {
		return 0, 0, fmt.Errorf("store: reconcile chapter refs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄

	var maxID int64
	bad := 0
	for rows.Next() {
		var id, blockID int64
		if err := rows.Scan(&id, &blockID); err != nil {
			return 0, 0, fmt.Errorf("store: reconcile scan chapter ref: %w", err)
		}
		maxID = id
		if _, ok := blocks[blockID]; !ok {
			bad++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("store: reconcile chapter refs rows: %w", err)
	}
	return maxID, bad, nil
}

// reconcileScanBatch 是悬空引用扫描的每批章节数（内存有界，不影响总成本）。
const reconcileScanBatch = 100000

// ---------- 健康报告用的快照 ----------

// ContentHealth 是 HealthReport 里的正文库摘要（最近一次对账快照 + 检查时间）。
//
// 对账本身是全表扫描（生产数十秒），已整体移出请求路径：这里读到的是后台刷新
// 留下的快照。Stale 为 true 表示「快照过期 / 还没有 / 上次刷新失败」，
// 数字可能旧或为 0，但接口照常 200（见 health_refresh.go）。
type ContentHealth struct {
	OrphanBlocks int    `json:"orphanBlocks"`
	OrphanBooks  int    `json:"orphanBooks"`
	DanglingRefs int    `json:"danglingRefs"`
	MissingTable bool   `json:"missingTable"`
	CheckedAt    string `json:"checkedAt"`
	Stale        bool   `json:"stale,omitempty"`
}

// String 让对账结果在日志里可读（CLI/运维排查用）。
func (r ContentReconcileReport) String() string {
	parts := []string{
		fmt.Sprintf("孤儿块 %d（%d 本书）", r.OrphanBlocks, r.OrphanBooks),
		fmt.Sprintf("悬空引用 %d", r.DanglingRefs),
	}
	if r.RemovedBlocks > 0 {
		parts = append(parts, fmt.Sprintf("已删除 %d", r.RemovedBlocks))
	}
	if r.MissingTable {
		parts = append(parts, "正文库缺 chapter_blocks（未迁移或文件丢失）")
	}
	return strings.Join(parts, "，")
}
