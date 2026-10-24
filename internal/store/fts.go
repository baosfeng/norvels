package store

import (
	"context"
	"fmt"
)

// Reindex 重建 FTS 索引（external content 表从 chapters 重新读取镜像列）。
func (s *Store) Reindex(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO chapters_fts(chapters_fts) VALUES('rebuild')"); err != nil {
		return fmt.Errorf("store: reindex rebuild: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO chapters_fts(chapters_fts) VALUES('optimize')"); err != nil {
		return fmt.Errorf("store: reindex optimize: %w", err)
	}
	return nil
}
