package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// 本文件是覆盖表回填**状态行**的读写（fingerprint_coverage_state 单行表）。
//
// 状态行是后台回填写入 / 健康报告判读 / 迁移初始化三方共用的唯一锚点：
//   complete     历史指纹是否已全部进覆盖表（false → 健康报告标 stale + backfilling）；
//   last_book_id 断点（下一批从它之后开始），进程被杀后重跑即续。
//
// 与数据一起放在正文库：判断「覆盖表可信不可信」必须与覆盖表同库同寿命，
// 放主库 settings 会在「只恢复了主库」的现场给出错误结论。

// coverageState 读出回填状态：complete 表示历史指纹是否都已进覆盖表；
// lastBookID 是断点（下一批从它之后开始）。状态行缺失按「未完成」处理。
func (s *Store) coverageState(ctx context.Context) (complete bool, lastBookID int64, err error) {
	ready, err := s.contentTableExistsNamed(ctx, coverageStateTable)
	if err != nil || !ready {
		return false, 0, err
	}
	var done int
	err = s.cdb.QueryRowContext(ctx,
		"SELECT complete, last_book_id FROM "+coverageStateTable+" WHERE id = 1").Scan(&done, &lastBookID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("store: read %s: %w", coverageStateTable, err)
	}
	return done != 0, lastBookID, nil
}

// setCoverageState 写回填状态（单行 upsert）。
func (s *Store) setCoverageState(ctx context.Context, complete bool, lastBookID int64) error {
	flag := 0
	if complete {
		flag = 1
	}
	_, err := s.cdb.ExecContext(ctx,
		"INSERT INTO "+coverageStateTable+"(id, complete, last_book_id, updated_at) VALUES (1, ?, ?, ?) "+
			"ON CONFLICT(id) DO UPDATE SET complete = excluded.complete, "+
			"last_book_id = excluded.last_book_id, updated_at = excluded.updated_at",
		flag, lastBookID, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: write %s: %w", coverageStateTable, err)
	}
	return nil
}

// initCoverageState 初始化回填状态行（只在缺失时插入，幂等）。
//
// hasFingerprints 为 false 表示库里还没有指纹行（新建库）：直接把 complete 置 1——
// 之后所有写入都由 replaceFingerprints 同事务维护覆盖表，不存在「历史待补」。
func (s *Store) initCoverageState(ctx context.Context, hasFingerprints bool) error {
	flag := 1
	if hasFingerprints {
		flag = 0
	}
	_, err := s.cdb.ExecContext(ctx,
		"INSERT OR IGNORE INTO "+coverageStateTable+"(id, complete, last_book_id, updated_at) "+
			"VALUES (1, ?, 0, ?)", flag, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: init %s: %w", coverageStateTable, err)
	}
	return nil
}

// isNoRows 是「查无此行」的判定（探测步骤共用，省得每处都 import errors 再比较）。
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
