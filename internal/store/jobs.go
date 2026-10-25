package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// jobColumns 是 ImportJob 的通用取数列。
const jobColumns = "id, source_dir, status, total, imported, skipped, failed, started_at, finished_at, message"

// scanImportJob 扫描一行任务记录。
func scanImportJob(sc rowScanner) (*ImportJob, error) {
	var (
		j        ImportJob
		started  string
		finished sql.NullString
	)
	if err := sc.Scan(&j.ID, &j.SourceDir, &j.Status, &j.Total, &j.Imported, &j.Skipped,
		&j.Failed, &started, &finished, &j.Message); err != nil {
		return nil, err
	}
	j.StartedAt, _ = parseTime(started)
	if finished.Valid && finished.String != "" {
		if t, err := parseTime(finished.String); err == nil {
			j.FinishedAt = &t
		}
	}
	return &j, nil
}

// CreateImportJob 新建导入任务记录（状态 running）。
func (s *Store) CreateImportJob(ctx context.Context, sourceDir string) (*ImportJob, error) {
	sourceDir = strings.TrimSpace(sourceDir)
	if sourceDir == "" {
		return nil, fmt.Errorf("%w: empty source dir", ErrInvalidArgument)
	}
	now := nowUTC()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO import_jobs(source_dir, status, started_at) VALUES (?, 'running', ?)",
		sourceDir, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("store: create import job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: create import job id: %w", err)
	}
	return &ImportJob{ID: id, SourceDir: sourceDir, Status: "running", StartedAt: now}, nil
}

// UpdateImportJob 更新导入任务进度/状态。
func (s *Store) UpdateImportJob(ctx context.Context, j *ImportJob) error {
	if j == nil || j.ID <= 0 {
		return fmt.Errorf("%w: invalid import job", ErrInvalidArgument)
	}
	status := strings.TrimSpace(j.Status)
	if status == "" {
		status = "running"
	}
	var finished any
	if j.FinishedAt != nil {
		finished = formatTime(*j.FinishedAt)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE import_jobs SET status = ?, total = ?, imported = ?, skipped = ?, failed = ?, "+
			"finished_at = ?, message = ? WHERE id = ?",
		status, j.Total, j.Imported, j.Skipped, j.Failed, finished, j.Message, j.ID)
	if err != nil {
		return fmt.Errorf("store: update import job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update import job rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: import job %d", ErrNotFound, j.ID)
	}
	return nil
}

// GetImportJob 读取单个任务（含最近 200 条失败明细）。
func (s *Store) GetImportJob(ctx context.Context, id int64) (*ImportJobDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: import job %d", ErrInvalidArgument, id)
	}
	row := s.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE id = ?", id)
	job, err := scanImportJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: import job %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get import job: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT file_path, reason, created_at FROM import_errors WHERE job_id = ? ORDER BY id ASC LIMIT 200", id)
	if err != nil {
		return nil, fmt.Errorf("store: list import errors: %w", err)
	}
	defer rows.Close()

	errs := make([]ImportError, 0, 8)
	for rows.Next() {
		var (
			ie      ImportError
			created string
		)
		if err := rows.Scan(&ie.FilePath, &ie.Reason, &created); err != nil {
			return nil, fmt.Errorf("store: scan import error: %w", err)
		}
		ie.CreatedAt, _ = parseTime(created)
		errs = append(errs, ie)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list import errors rows: %w", err)
	}
	failed, err := s.CountImportJobFiles(ctx, id, ImportFileStatusFailed)
	if err != nil {
		return nil, err
	}
	return &ImportJobDetail{ImportJob: *job, Errors: errs, FailedCount: failed}, nil
}

// 文件级终态的取值（0012 起，与 spec §2.1 一致）。
const (
	ImportFileStatusImported    = "imported"
	ImportFileStatusFailed      = "failed"
	ImportFileStatusSkipped     = "skipped"
	ImportFileStatusUnsupported = "unsupported"
	ImportFileStatusCanceled    = "canceled"
)

// importFileStatuses 是允许落库/筛选的终态集合（白名单：非法值一律拒收）。
var importFileStatuses = map[string]bool{
	ImportFileStatusImported:    true,
	ImportFileStatusFailed:      true,
	ImportFileStatusSkipped:     true,
	ImportFileStatusUnsupported: true,
	ImportFileStatusCanceled:    true,
}

// ValidImportFileStatus 报告值是否是合法的文件级终态。
func ValidImportFileStatus(status string) bool { return importFileStatuses[status] }

// UpsertImportJobFile 写一条文件级终态；同一 (job_id, source) 原地更新。
//
// 幂等的用途：重试/重放同一个源时不堆重复行，状态以最后一次为准。
// 任务不存在返回 ErrNotFound（不写孤儿行）；源为空或状态非法返回 ErrInvalidArgument。
func (s *Store) UpsertImportJobFile(ctx context.Context, jobID int64, f ImportJobFile) error {
	if jobID <= 0 {
		return fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	source := strings.TrimSpace(f.Source)
	status := strings.TrimSpace(f.Status)
	if source == "" || !ValidImportFileStatus(status) {
		return fmt.Errorf("%w: source=%q status=%q", ErrInvalidArgument, f.Source, f.Status)
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM import_jobs WHERE id = ?)", jobID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check import job %d: %w", jobID, err)
	}
	if !exists {
		return fmt.Errorf("%w: import job %d", ErrNotFound, jobID)
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO import_job_files(job_id, source, display_path, status, reason, book_id, updated_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?) "+
			"ON CONFLICT(job_id, source) DO UPDATE SET display_path=excluded.display_path, "+
			"status=excluded.status, reason=excluded.reason, book_id=excluded.book_id, "+
			"updated_at=excluded.updated_at",
		jobID, source, strings.TrimSpace(f.DisplayPath), status, f.Reason, f.BookID,
		formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: upsert import job file: %w", err)
	}
	return nil
}

// ListImportJobFiles 列出某个任务的文件级终态；status 为空表示全部。
//
// limit 由调用方给定（api 层用 1000）；查询 limit+1 行判断是否被截断，
// 避免「刚好 1000 条」时误报 truncated。
func (s *Store) ListImportJobFiles(ctx context.Context, jobID int64, status string, limit int) (ImportJobFiles, error) {
	if err := s.checkImportJobFileQuery(ctx, jobID, status); err != nil {
		return ImportJobFiles{}, err
	}
	if limit <= 0 {
		limit = maxImportJobFiles
	}
	files, err := s.queryImportJobFiles(ctx, jobID, status, limit+1)
	if err != nil {
		return ImportJobFiles{}, err
	}
	out := ImportJobFiles{Files: files}
	if len(files) > limit {
		out.Files = files[:limit]
		out.Truncated = true
	}
	return out, nil
}

// checkImportJobFileQuery 校验列表入参：任务存在、状态在白名单内。
func (s *Store) checkImportJobFileQuery(ctx context.Context, jobID int64, status string) error {
	if jobID <= 0 {
		return fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	if status != "" && !ValidImportFileStatus(status) {
		return fmt.Errorf("%w: import file status %q", ErrInvalidArgument, status)
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM import_jobs WHERE id = ?)", jobID).Scan(&exists); err != nil {
		return fmt.Errorf("store: check import job %d: %w", jobID, err)
	}
	if !exists {
		return fmt.Errorf("%w: import job %d", ErrNotFound, jobID)
	}
	return nil
}

// queryImportJobFiles 执行文件级终态查询（limit 已含 +1 的截断探测）。
func (s *Store) queryImportJobFiles(ctx context.Context, jobID int64, status string, limit int) ([]ImportJobFile, error) {
	status = strings.TrimSpace(status)
	q := "SELECT id, source, display_path, status, reason, book_id, updated_at FROM import_job_files WHERE job_id = ?"
	args := []any{jobID}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	q += " ORDER BY id LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list import job files: %w", err)
	}
	defer rows.Close()

	out := make([]ImportJobFile, 0, 16)
	for rows.Next() {
		var (
			f       ImportJobFile
			updated string
		)
		if err := rows.Scan(&f.ID, &f.Source, &f.DisplayPath, &f.Status, &f.Reason,
			&f.BookID, &updated); err != nil {
			return nil, fmt.Errorf("store: scan import job file: %w", err)
		}
		f.UpdatedAt, _ = parseTime(updated)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list import job files rows: %w", err)
	}
	return out, nil
}

// maxImportJobFiles 是文件级终态列表的上限（与 docs/api.md 一致）。
const maxImportJobFiles = 1000

// CountImportJobFiles 数某个任务里指定状态的终态条数（空字符串数全部）。
func (s *Store) CountImportJobFiles(ctx context.Context, jobID int64, status string) (int, error) {
	if jobID <= 0 {
		return 0, fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	status = strings.TrimSpace(status)
	q := "SELECT COUNT(*) FROM import_job_files WHERE job_id = ?"
	args := []any{jobID}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count import job files: %w", err)
	}
	return n, nil
}

// ImportJobFileStatuses 返回指定任务里每个 source 的终态（不存在的 source 不在 map 里）。
//
// 重试入口用它校验「源属于该任务且可重试」：不能只看列表（可能被 truncated 截断）。
func (s *Store) ImportJobFileStatuses(ctx context.Context, jobID int64, sources []string) (map[string]string, error) {
	if jobID <= 0 {
		return nil, fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	out := make(map[string]string, len(sources))
	if len(sources) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT source, status FROM import_job_files WHERE job_id = ?", jobID)
	if err != nil {
		return nil, fmt.Errorf("store: list import job file statuses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var source, status string
		if err := rows.Scan(&source, &status); err != nil {
			return nil, fmt.Errorf("store: scan import job file status: %w", err)
		}
		out[source] = status
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list import job file statuses rows: %w", err)
	}
	return out, nil
}

// FailedImportJobFileSources 返回某任务里 status=failed 的可重放源（按行序）。
func (s *Store) FailedImportJobFileSources(ctx context.Context, jobID int64) ([]string, error) {
	if jobID <= 0 {
		return nil, fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT source FROM import_job_files WHERE job_id = ? AND status = ? ORDER BY id",
		jobID, ImportFileStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("store: list failed import job files: %w", err)
	}
	defer rows.Close()

	sources := make([]string, 0, 8)
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, fmt.Errorf("store: scan failed import job file: %w", err)
		}
		sources = append(sources, src)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list failed import job files rows: %w", err)
	}
	return sources, nil
}

// ListImportJobs 列出最近的任务（默认 20 条，上限 200）。
func (s *Store) ListImportJobs(ctx context.Context, limit int) ([]ImportJob, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+jobColumns+" FROM import_jobs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("store: list import jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]ImportJob, 0, limit)
	for rows.Next() {
		job, err := scanImportJob(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan import job: %w", err)
		}
		jobs = append(jobs, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list import jobs rows: %w", err)
	}
	return jobs, nil
}

// AddImportError 记录单文件失败明细。
func (s *Store) AddImportError(ctx context.Context, jobID int64, filePath, reason string) error {
	if jobID <= 0 {
		return fmt.Errorf("%w: import job %d", ErrInvalidArgument, jobID)
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO import_errors(job_id, file_path, reason, created_at) VALUES (?, ?, ?, ?)",
		jobID, filePath, reason, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: add import error: %w", err)
	}
	return nil
}
