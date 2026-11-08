package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RollbackEnrichBatch 按批次把旧值写回，返回回滚的变更条数。
//
// 三条口径：
//   - 只回滚本次记录过的字段，不做「整本书恢复快照」——那是另一种语义，
//     而且嵌入式的正文库无法原子地跟着回退；
//   - 逐条走 UpdateBookMeta（作者同步 books_authors 与聚合列），不写裸 SQL；
//   - 单条失败不中断整批：路径上已有 book 被删、字段被用户改过等情形都会让单条失败，
//     中断会让「回滚一半」这种最难收拾的状态出现；失败条数由返回值的差集体现。
func (s *Store) RollbackEnrichBatch(ctx context.Context, batchID string) (int, error) {
	changes, err := s.EnrichBatchChanges(ctx, batchID)
	if err != nil {
		return 0, err
	}
	if len(changes) == 0 {
		return 0, fmt.Errorf("%w: 批次 %s 没有变更记录", ErrInvalidArgument, batchID)
	}
	done := 0
	var firstErr error
	for _, ch := range changes {
		patch, ok := rollbackPatch(ch.Field, ch.OldValue)
		if !ok {
			continue
		}
		if err := s.UpdateBookMeta(ctx, ch.BookID, patch); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		done++
	}
	if firstErr != nil {
		return done, fmt.Errorf("store: 回滚部分失败（成功 %d / 共 %d）：%w", done, len(changes), firstErr)
	}
	return done, nil
}

// rollbackPatch 把一条变更记录翻成「写回旧值」的补丁；未知字段返回 ok=false。
func rollbackPatch(field, old string) (BookMetaPatch, bool) {
	v := old
	switch field {
	case EnrichFieldAuthor:
		return BookMetaPatch{Author: &v}, true
	case EnrichFieldIntro:
		return BookMetaPatch{Intro: &v}, true
	case EnrichFieldCategory:
		l1, l2 := splitCategoryValue(v)
		return BookMetaPatch{CategoryL1: &l1, CategoryL2: &l2}, true
	}
	return BookMetaPatch{}, false
}

// splitCategoryValue 把 "L1/L2" 拆回两级；没有 "/" 时整串当 L1。
func splitCategoryValue(v string) (l1, l2 string) {
	if i := strings.IndexByte(v, '/'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// RetryOnLocked 把一个写操作在遇到 SQLITE_BUSY / database is locked 时重试若干次。
//
// 为什么需要：enrich 这类运维命令是在**服务照常运行时**执行的，两边都要写主库；
// busy_timeout(10s) 挡得住短时争用，但一次批量写里偶尔会有超过 10s 的窗口
// （服务侧的导入/回填任务）。重试让「碰到争用」不再等于「这批有一半没写进去」。
func RetryOnLocked(ctx context.Context, attempts int, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil || !isLockedErr(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 500 * time.Millisecond):
		}
	}
	return err
}

// isLockedErr 判定是否属于「被别的写者占着」这类可重试错误。
func isLockedErr(err error) bool {
	if err == nil {
		return false
	}
	if err == sql.ErrConnDone {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy")
}
