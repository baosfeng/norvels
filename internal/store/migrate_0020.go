package store

import (
	"context"
	"database/sql"
	"log"
)

// 0020 的主库步骤：给 books 补七列质量分（设计 §5）。
//
// 与 0008/0010/0011/0014 同一条路径：SQLite 没有 ADD COLUMN IF NOT EXISTS，
// 「删掉版本号重跑」是排查迁移问题的常规手段，撞 duplicate column 会让整次迁移失败，
// 因此先探测列是否存在、再决定执行哪条 ALTER，且跑在 0020 自己的事务里。
//
// 存量行保持零值（quality_chapters = 0、quality_at = ”）= 未计算，不猜：
// 质量分只能从正文算出来，而回填要解压几十 GB 正文，属于离线运维动作
// （norvels backfill-quality，见 quality_backfill.go）。
func addBookQualityColumnsStep(ctx context.Context, tx *sql.Tx) error {
	for _, c := range bookQualityColumns {
		if err := addColumnIfMissing(ctx, tx, "books", c.name, c.ddl); err != nil {
			return err
		}
	}
	log.Printf("迁移 0020：books 已补 %d 列质量分（存量行未计算，按需跑 norvels backfill-quality）",
		len(bookQualityColumns))
	return nil
}
