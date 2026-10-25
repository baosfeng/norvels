package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// looseVolumeTitle 是未归卷章节所在伪卷的标题。
const looseVolumeTitle = "未分卷"

// bookDetailRow 是 GetBook 主体行的扫描结果（在 BookSummary 之外的详情字段）。
type bookDetailRow struct {
	summary    BookSummary
	encoding   string
	sourceName string
	fileSize   int64
}

// GetBook 返回书籍详情（含完整目录树）。未归卷章节包装成伪卷 id=0/idx=-1。
func (s *Store) GetBook(ctx context.Context, id int64) (*BookDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: book id %d", ErrInvalidArgument, id)
	}
	row, err := s.loadBookDetailRow(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &BookDetail{
		BookSummary: row.summary,
		Encoding:    row.encoding,
		SourceName:  row.sourceName,
		FileSize:    row.fileSize,
		Volumes:     make([]VolumeNode, 0, 8),
	}

	authors, err := s.loadBookAuthors(ctx, id, row.summary.AuthorName)
	if err != nil {
		return nil, err
	}
	detail.Authors = authors

	tags, err := s.ListBookTags(ctx, id)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []string{}
	}
	detail.Tags = tags

	volIndex, err := s.loadVolumeNodes(ctx, id, detail)
	if err != nil {
		return nil, err
	}
	loose, err := s.loadChapterBriefs(ctx, id, volIndex, detail)
	if err != nil {
		return nil, err
	}
	if len(loose) > 0 {
		detail.Volumes = append(detail.Volumes, VolumeNode{
			ID:       0,
			Idx:      -1,
			Title:    looseVolumeTitle,
			Chapters: loose,
		})
	}
	return detail, nil
}

// loadBookDetailRow 读取书籍主体行并扫描成 BookSummary + 详情字段。
func (s *Store) loadBookDetailRow(ctx context.Context, id int64) (bookDetailRow, error) {
	var out bookDetailRow
	var (
		imported string
		updated  string
	)
	row := s.db.QueryRowContext(ctx,
		"SELECT "+bookSummaryColumns+", b.encoding, b.source_name, b.file_size "+
			"FROM books b LEFT JOIN authors a ON a.id = b.author_id WHERE b.id = ?", id)
	// 摘要列 + 详情列共用一套扫描目标：列顺序由 bookSummaryColumns 单点决定。
	dest := append(summaryScanTargets(&out.summary, &imported, &updated),
		&out.encoding, &out.sourceName, &out.fileSize)
	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("%w: book %d", ErrNotFound, id)
	}
	if err != nil {
		return out, fmt.Errorf("store: get book: %w", err)
	}
	applySummaryTimes(&out.summary, imported, updated)
	return out, nil
}

// loadVolumeNodes 载入卷列表并追加到 detail.Volumes，返回「库内卷 id → 卷下标」映射。
func (s *Store) loadVolumeNodes(ctx context.Context, bookID int64, detail *BookDetail) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, title FROM volumes WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: get book volumes: %w", err)
	}
	defer rows.Close()

	volIndex := make(map[int64]int, 8)
	for rows.Next() {
		var v VolumeNode
		if err := rows.Scan(&v.ID, &v.Idx, &v.Title); err != nil {
			return nil, fmt.Errorf("store: scan volume: %w", err)
		}
		v.Chapters = make([]ChapterBrief, 0, 32)
		volIndex[v.ID] = len(detail.Volumes)
		detail.Volumes = append(detail.Volumes, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get book volumes rows: %w", err)
	}
	return volIndex, nil
}

// loadChapterBriefs 载入章节目录，按 volIndex 归入对应卷；返回未归卷的章节（伪卷内容）。
func (s *Store) loadChapterBriefs(ctx context.Context, bookID int64, volIndex map[int64]int, detail *BookDetail) ([]ChapterBrief, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, title, volume_id, word_count FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: get book chapters: %w", err)
	}
	defer rows.Close()

	loose := make([]ChapterBrief, 0, 8)
	for rows.Next() {
		var c ChapterBrief
		if err := rows.Scan(&c.ID, &c.Idx, &c.Title, &c.VolumeID, &c.WordCount); err != nil {
			return nil, fmt.Errorf("store: scan chapter brief: %w", err)
		}
		if idx, ok := volIndex[c.VolumeID]; ok {
			detail.Volumes[idx].Chapters = append(detail.Volumes[idx].Chapters, c)
			continue
		}
		loose = append(loose, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: get book chapters rows: %w", err)
	}
	return loose, nil
}
