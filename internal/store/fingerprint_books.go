package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// 本文件是正文库「书级指纹覆盖表」fingerprint_books 的唯一口径。
//
// 表结构在 content.go 的 contentSchemaDDL（0023 建它）；维护只发生在写指纹的同一事务里
// （content_fingerprint.go 的 replaceFingerprints / purgeBookFingerprints）；
// 健康报告只对它做 COUNT(*)，不再对 chapter_fingerprints 做 COUNT(DISTINCT book_id)。
//
// 为什么必须换：health/report 原先那条 COUNT(DISTINCT book_id) 在正文库
// 51,254,165 行上实测约 22 s，是 /api/health/report 稳定 21 s 的唯一大头，
// 且 IO 竞争下会退化到 60 s 超时；而前端驾驶舱的 STATS_TIMEOUT_MS = 20_000，
// 也就是说 21 s 的响应在驾驶舱里**本来就是超时失败**——健康卡片永远拿不到数据。
// 覆盖表把这件事变成 O(书数)（生产 16.9 万本，毫秒级）。
//
// 历史库的 5125 万行指纹怎么补：**不在迁移里补**（AGENTS.md：大库迁移 20+ 分钟、
// 期间不监听端口），而是迁移只建表，启动后由后台分批任务扫描入库（可中断、可续跑，
// 断点记在 fingerprint_coverage_state.last_book_id）。回填完成前健康报告标 stale。
//
// 覆盖表与指纹行仍可能因裸 SQL 漂移：RepairFingerprintCoverage 全量重算，
// 「增量 == 全量」由 TestFingerprintCoverageMatchesFullRecount 守着。

// fingerprintBooksTable / coverageStateTable 是两张表的名字。
const (
	fingerprintBooksTable = "fingerprint_books"
	coverageStateTable    = "fingerprint_coverage_state"
	// coverageBackfillBatch 是后台回填每批处理的书数：批越大越快，但中断丢的进度越多。
	// 生产 16.9 万本 → 34 批；每批一次索引区间扫描 + 一次多行 INSERT。
	coverageBackfillBatch = 5000
	// coverageBackfillTimeout 是单次后台回填的上限：卡住的回填不许永久占住 CAS。
	coverageBackfillTimeout = 30 * time.Minute
)

// fingerprintBooksTableExists 判断正文库有没有覆盖表（0023 之前的库没有）。
func (s *Store) fingerprintBooksTableExists(ctx context.Context) (bool, error) {
	return s.contentTableExistsNamed(ctx, fingerprintBooksTable)
}

// contentTableExistsNamed 判断正文库里有没有指定表（覆盖表 / 状态表共用）。
func (s *Store) contentTableExistsNamed(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check %s: %w", name, err)
	}
	return n > 0, nil
}

// fingerprintBooksTableExistsInTx 是事务内版本（写路径要在同一事务里决定维不维护覆盖表）。
func fingerprintBooksTableExistsInTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		fingerprintBooksTable).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check %s in tx: %w", fingerprintBooksTable, err)
	}
	return n > 0, nil
}

// fingerprintCoverageCount 返回有指纹的书数（覆盖表行数，毫秒级）。
func (s *Store) fingerprintCoverageCount(ctx context.Context) (int, error) {
	var n int
	if err := s.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+fingerprintBooksTable).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count %s: %w", fingerprintBooksTable, err)
	}
	return n, nil
}

// setFingerprintCoverage 把一本书的覆盖状态写成 has（与指纹行的增删同事务）。
//
// 表不存在（0023 之前的库）时静默跳过：那时写指纹本身仍然有效，只是健康报告
// 走 COUNT(DISTINCT) 兜底并标 stale——不能因为一张辅助表没建就让整批导入失败。
func setFingerprintCoverage(ctx context.Context, tx *sql.Tx, bookID int64, has bool) error {
	if bookID <= 0 {
		return nil
	}
	ready, err := fingerprintBooksTableExistsInTx(ctx, tx)
	if err != nil || !ready {
		return err
	}
	if has {
		_, err = tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO "+fingerprintBooksTable+"(book_id) VALUES (?)", bookID)
	} else {
		_, err = tx.ExecContext(ctx,
			"DELETE FROM "+fingerprintBooksTable+" WHERE book_id = ?", bookID)
	}
	if err != nil {
		return fmt.Errorf("store: set fingerprint coverage book=%d: %w", bookID, err)
	}
	return nil
}

// FingerprintCoverageReport 是一次覆盖表重建的汇总（CLI 与后台日志共用）。
type FingerprintCoverageReport struct {
	Covered int // 完成后覆盖的书数
	Added   int // 本次补进覆盖表的书数
	Pruned  int // 本次清掉的「有覆盖行但没有指纹行」的陈旧书数
	Elapsed time.Duration
}

// BackfillFingerprintCoverage 把历史指纹分批补进覆盖表（启动后台任务与运维命令共用）。
//
// 语义与 0019 的 backfill-fingerprints 同一套：跳过式、按批落盘、可中断续跑。
//   - 从状态行里的 last_book_id 之后继续，每批 coverageBackfillBatch 本书；
//   - 每批一次多行 INSERT OR IGNORE，写完就把断点落盘——进程被杀最多重做一批；
//   - 扫完之后清掉陈旧覆盖行，最后才把 complete 置 1（健康报告据此停止标 stale）。
//
// 成本：正文库 chapter_fingerprints 的 book_id 索引扫一遍（生产 5125 万项），
// 每 5000 本书一次插入。实测速率见 docs 与迁移日志（回填只在后台跑，不挡监听）。
func (s *Store) BackfillFingerprintCoverage(ctx context.Context) (FingerprintCoverageReport, error) {
	start := time.Now()
	var rep FingerprintCoverageReport
	ready, err := s.fingerprintBooksTableExists(ctx)
	if err != nil {
		return rep, err
	}
	if !ready {
		return rep, fmt.Errorf("store: 正文库没有 %s（先跑 norvels migrate 应用 0023）", fingerprintBooksTable)
	}
	_, lastID, err := s.coverageState(ctx)
	if err != nil {
		return rep, err
	}
	added, lastID, err := s.backfillCoverageBatches(ctx, lastID)
	rep.Added = added
	if err != nil {
		return rep, err // 断点已按批落盘，重跑从这里继续
	}
	pruned, err := s.pruneCoverageRows(ctx)
	if err != nil {
		return rep, err
	}
	rep.Pruned = pruned
	if rep.Covered, err = s.fingerprintCoverageCount(ctx); err != nil {
		return rep, err
	}
	if err := s.setCoverageState(ctx, true, lastID); err != nil {
		return rep, err
	}
	rep.Elapsed = time.Since(start)
	return rep, nil
}

// backfillCoverageBatches 逐批搬运，返回本次新增的书数与最后的断点。
func (s *Store) backfillCoverageBatches(ctx context.Context, lastID int64) (int, int64, error) {
	added := 0
	for {
		if err := ctx.Err(); err != nil {
			return added, lastID, err //nolint:wrapcheck // 取消原样上抛，断点已落盘
		}
		ids, err := s.fingerprintBooksAfter(ctx, lastID, coverageBackfillBatch)
		if err != nil {
			return added, lastID, err
		}
		if len(ids) == 0 {
			return added, lastID, nil
		}
		n, err := s.insertCoverageRows(ctx, ids)
		if err != nil {
			return added, lastID, err
		}
		added += n
		lastID = ids[len(ids)-1]
		// 先落盘断点再继续：中断后重跑从这一批之后开始。
		if err := s.setCoverageState(ctx, false, lastID); err != nil {
			return added, lastID, err
		}
	}
}

// fingerprintBooksAfter 取 lastID 之后最多 limit 个「有指纹的书 id」（升序）。
//
// WHERE book_id > ? ORDER BY book_id LIMIT n 走 idx_chapter_fingerprints_book 的区间扫描，
// 每个值只被返回一次（DISTINCT 在索引上前缀去重），不会把整张表读进内存。
func (s *Store) fingerprintBooksAfter(ctx context.Context, lastID int64, limit int) ([]int64, error) {
	rows, err := s.cdb.QueryContext(ctx,
		"SELECT DISTINCT book_id FROM chapter_fingerprints WHERE book_id > ? ORDER BY book_id LIMIT ?",
		lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: scan fingerprint books after %d: %w", lastID, err)
	}
	return scanIDs(rows)
}

// insertCoverageRows 一次写一批覆盖行，返回真正新增的行数（已存在的被 IGNORE）。
func (s *Store) insertCoverageRows(ctx context.Context, ids []int64) (int, error) {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.cdb.ExecContext(ctx,
		"INSERT OR IGNORE INTO "+fingerprintBooksTable+"(book_id) VALUES "+valuesPlaceholders(len(ids)),
		args...)
	if err != nil {
		return 0, fmt.Errorf("store: insert %d coverage rows: %w", len(ids), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil //nolint:nilerr // 驱动不报行数时只是少了统计，写入已生效
	}
	return int(n), nil
}

// valuesPlaceholders 生成多行 VALUES 的占位符 "(?), (?), …"（n 行、每行一列）。
func valuesPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("(?),", n), ",")
}

// RepairFingerprintCoverage 从头重算覆盖表（运维入口 --coverage-only、可重入）。
//
// 与后台回填是同一份实现，只是先把断点归零：运维要用的是「以指纹行为准的完整重算」，
// 不是「从上次断点继续」。两条方向都会修：有指纹没覆盖行 → 补，有覆盖行没指纹 → 清。
func (s *Store) RepairFingerprintCoverage(ctx context.Context) (FingerprintCoverageReport, error) {
	var rep FingerprintCoverageReport
	ready, err := s.fingerprintBooksTableExists(ctx)
	if err != nil {
		return rep, err
	}
	if !ready {
		return rep, fmt.Errorf("store: 正文库没有 %s（先跑 norvels migrate 应用 0023）", fingerprintBooksTable)
	}
	if err := s.setCoverageState(ctx, false, 0); err != nil {
		return rep, err
	}
	return s.BackfillFingerprintCoverage(ctx)
}

// EnsureFingerprintCoverage 在「回填还没完成」时启动后台回填（启动路径与健康读取都可调）。
//
// 非阻塞、可重复调用：真正的去重在 StartFingerprintBackfill 的 atomic CAS 里。
func (s *Store) EnsureFingerprintCoverage(ctx context.Context) {
	ready, err := s.fingerprintBooksTableExists(ctx)
	if err != nil || !ready {
		return
	}
	complete, _, err := s.coverageState(ctx)
	if err != nil || complete {
		return
	}
	s.StartFingerprintBackfill(ctx)
}

// StartFingerprintBackfill 启动后台回填：不阻塞调用方（启动流程 / 请求路径都不等它）。
//
// 同一时刻只允许一个回填（atomic CAS，与 metricsRefreshing 同一口径）；
// 中断只记一行 WARN，状态行留着断点，下次启动接着补。
func (s *Store) StartFingerprintBackfill(ctx context.Context) {
	b := &s.reconcile
	if !b.coverageRunning.CompareAndSwap(false, true) {
		return
	}
	base := s.backgroundContext() // 与请求生命周期无关，Stop 时统一取消
	go func() {
		defer b.coverageRunning.Store(false)
		rctx, cancel := context.WithTimeout(base, coverageBackfillTimeout)
		defer cancel()
		rep, err := s.BackfillFingerprintCoverage(rctx)
		if err != nil {
			log.Printf("WARN: 指纹覆盖表后台回填中断（断点已保存，下次启动接着补）: %v", err)
			return
		}
		log.Printf("指纹覆盖表回填完成：覆盖 %d 本书（补 %d / 清理陈旧 %d，耗时 %v）",
			rep.Covered, rep.Added, rep.Pruned, rep.Elapsed.Round(time.Millisecond))
	}()
}

// CoverageBackfillRunning 报告后台回填是否在跑（健康诊断与测试用）。
func (s *Store) CoverageBackfillRunning() bool { return s.reconcile.coverageRunning.Load() }

// pruneCoverageRows 删掉「有覆盖行但没有一条指纹」的书，返回删了几本。
//
// 为什么不写一句 DELETE ... WHERE book_id NOT IN (SELECT book_id FROM chapter_fingerprints)：
// 那会把 5125 万行的指纹表当子查询跑一遍（正是本次要消灭的全表扫）。
// 覆盖表只有书数量级，逐本按 idx_chapter_fingerprints_book 做一次 LIMIT 1 探测即可。
func (s *Store) pruneCoverageRows(ctx context.Context) (int, error) {
	ids, err := s.coveredBookIDs(ctx)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return pruned, err //nolint:wrapcheck // 取消原样上抛，已删的保留
		}
		has, err := s.bookHasFingerprints(ctx, id)
		if err != nil {
			return pruned, err
		}
		if has {
			continue
		}
		if _, err := s.cdb.ExecContext(ctx,
			"DELETE FROM "+fingerprintBooksTable+" WHERE book_id = ?", id); err != nil {
			return pruned, fmt.Errorf("store: prune %s book=%d: %w", fingerprintBooksTable, id, err)
		}
		pruned++
	}
	return pruned, nil
}

// coveredBookIDs 读出覆盖表里的全部书 id（书数量级，内存有界）。
func (s *Store) coveredBookIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.cdb.QueryContext(ctx, "SELECT book_id FROM "+fingerprintBooksTable)
	if err != nil {
		return nil, fmt.Errorf("store: list %s: %w", fingerprintBooksTable, err)
	}
	return scanIDs(rows)
}

// bookHasFingerprints 判断一本书在正文库里还有没有指纹行（走 book_id 索引）。
func (s *Store) bookHasFingerprints(ctx context.Context, bookID int64) (bool, error) {
	var one int
	err := s.cdb.QueryRowContext(ctx,
		"SELECT 1 FROM chapter_fingerprints WHERE book_id = ? LIMIT 1", bookID).Scan(&one)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	default:
		return false, fmt.Errorf("store: probe fingerprints book=%d: %w", bookID, err)
	}
}
