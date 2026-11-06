package store

import (
	"context"
	"database/sql"
	"log"
)

// 0023 的跨库步骤：正文库建书级指纹覆盖表 fingerprint_books + 回填状态表。
//
// **本步骤不做回填**，只建表并把状态行置成「待回填」或「无需回填」：
// 生产的 chapter_fingerprints 有 5,125 万行，全量补进覆盖表要扫一遍 book_id 索引
// （分钟级）。AGENTS.md 的硬约束是「大库迁移 20+ 分钟且期间不监听端口」，
// 把这种扫描塞进迁移等于让每次升级都多停服几分钟，且中途被杀还要从头再来。
//
// 所以回填改成启动后的**后台分批任务**（BackfillFingerprintCoverage）：
//
//	· 迁移只花建表的毫秒级时间，进程立刻开始监听；
//	· 回填每批 5000 本书落一次断点，可中断、可续跑，不挡任何请求；
//	· 回填完成前健康报告的指纹覆盖数标 stale + backfilling（不让「还没补到」被读成「都没有指纹」）。
//
// 判断「要不要回填」只看一次 LIMIT 1 探测（O(1)）：指纹表一行都没有的库（新建库）
// 直接置 complete=1——之后所有写入都由 replaceFingerprints 同事务维护覆盖表。
//
// 幂等：建表 IF NOT EXISTS；状态行只在缺失时插入，重跑不会把已完成的库打回待回填。
func createFingerprintCoverageStep(ctx context.Context, _ *sql.Tx, s *Store) error {
	if err := s.ensureContentSchema(ctx); err != nil {
		return err
	}
	hasRows, err := s.hasFingerprintRows(ctx)
	if err != nil {
		return err
	}
	if err := s.initCoverageState(ctx, hasRows); err != nil {
		return err
	}
	log.Printf("迁移 0023：正文库已建 %s（历史指纹回填 %s）", fingerprintBooksTable,
		coveragePlanText(hasRows))
	return nil
}

// hasFingerprintRows 判断正文库指纹表里有没有行（LIMIT 1，O(1)）。
//
// 不用 COUNT(*)：5125 万行上那是秒级全表扫，而这里只关心「空不空」。
func (s *Store) hasFingerprintRows(ctx context.Context) (bool, error) {
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil || !ready {
		return false, err
	}
	var one int
	err = s.cdb.QueryRowContext(ctx, "SELECT 1 FROM chapter_fingerprints LIMIT 1").Scan(&one)
	switch {
	case err == nil:
		return true, nil
	case isNoRows(err):
		return false, nil
	default:
		return false, err
	}
}

// coveragePlanText 是迁移日志里的回填计划文案（运维据此知道接下来会不会有后台任务）。
func coveragePlanText(hasRows bool) string {
	if hasRows {
		return "交给启动后的后台任务分批进行（不阻塞监听，可中断续跑）"
	}
	return "无需回填（库里还没有指纹行）"
}
