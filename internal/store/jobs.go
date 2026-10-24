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
	return &ImportJobDetail{ImportJob: *job, Errors: errs}, nil
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
