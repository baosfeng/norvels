package store

import (
	"context"
	"fmt"
	"strings"
)

// BookExistsForImport 复刻 SaveBook 的「已存在」判定，供 dry-run 预检使用。
//
// 判定顺序与真实导入一致：source_path 或 file_sha256 命中即已存在；
// 追更模式（update）在前两级都没命中时降级到「书名 + 作者」指纹兜底。
// force 覆盖不算重复由调用方处理（force 与 update 互斥优先级见 SaveOptions）。
func (s *Store) BookExistsForImport(ctx context.Context, sourcePath, sha, title, author string, update bool) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM books WHERE source_path = ? OR file_sha256 = ?)",
		strings.TrimSpace(sourcePath), strings.TrimSpace(sha)).Scan(&exists); err != nil {
		return false, fmt.Errorf("store: exists for import: %w", err)
	}
	if exists || !update {
		return exists, nil
	}
	if err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM books b LEFT JOIN authors a ON a.id = b.author_id "+
			"WHERE b.title = ? AND COALESCE(a.name, '') = ?)",
		strings.TrimSpace(title), strings.TrimSpace(author)).Scan(&exists); err != nil {
		return false, fmt.Errorf("store: exists by title/author: %w", err)
	}
	return exists, nil
}
