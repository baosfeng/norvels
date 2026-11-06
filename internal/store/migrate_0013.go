package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// 0013 的迁移步骤：先把同书重名卷改名去重，再建唯一索引（表达式索引 TRIM(title)）。
//
// 从 migrate_steps.go 拆出来：那个文件已顶到门禁 5 的 400 行硬上限，
// 而这里的内容是 0013 的自洽一段（规划改名 → 执行改名 → 建索引），单独成文件更好读。
// ---------- 0013：同书卷名唯一索引 ----------

// volumeTitleUniqueIndex 是 0013 建的同书卷名唯一索引名（表达式索引，TRIM(title)）。
//
// 同一个名字有两个用途：迁移里建它；写入口把「撞这个索引」的错误翻成
// ErrVolumeNameTaken（索引是并发竞态的兜底，正常路径仍走事务内查重）。
const volumeTitleUniqueIndex = "uq_volumes_book_title"

// volumeRow 是去重规划需要的一行卷：id、所属书、库里 TRIM 后的标题。
type volumeRow struct {
	ID    int64
	Book  int64
	Title string
}

// volumeRename 是一条待执行的改名。
type volumeRename struct {
	ID    int64
	Title string
}

// dedupeVolumeTitlesStep 是 0013 的全部内容：先把同书重名卷改名去重，再建唯一索引。
//
// 为什么整段放在 Go 步骤里而不写进迁移 SQL：补序号要先知道本书已有哪些卷名，
// 才能保证新名字不会又撞上真实存在的「原标题 (2)」；纯 SQL 没有有界探测，
// 而改名撞上唯一约束会让整条迁移回滚、服务起不来。
//
// 改名条数走 log.Printf 打进迁移日志：与 cmd 的「已应用 0013 …」同一个 sink，
// 运维升级完直接能看到这台机器改了几条，不需要再去查库。
func dedupeVolumeTitlesStep(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableExistsInTx(ctx, tx, "volumes")
	if err != nil || !exists {
		return err // 老库升级路径上 volumes 可能还不存在
	}
	rows, err := loadVolumeRows(ctx, tx)
	if err != nil {
		return err
	}
	renames := planVolumeRenames(rows)
	if err := applyVolumeRenames(ctx, tx, renames); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"CREATE UNIQUE INDEX IF NOT EXISTS "+volumeTitleUniqueIndex+
			" ON volumes(book_id, TRIM(title))"); err != nil {
		return fmt.Errorf("create unique index %s: %w", volumeTitleUniqueIndex, err)
	}
	log.Printf("迁移 0013：同书卷名去重改名 %d 条，已建唯一索引 %s",
		len(renames), volumeTitleUniqueIndex)
	return nil
}

// loadVolumeRows 按 (book_id, idx, id) 读出全部卷 id 与 TRIM 后标题。
//
// 排序口径写死 (idx, id)：去重时「保留首个」必须可复现，不能随行的物理顺序变。
// 标题取 SQL 的 TRIM 值而不是 Go 的 TrimSpace——唯一索引按 SQL TRIM 建，
// 两边口径必须完全一致。
func loadVolumeRows(ctx context.Context, tx *sql.Tx) ([]volumeRow, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, book_id, TRIM(title) FROM volumes ORDER BY book_id, idx, id")
	if err != nil {
		return nil, fmt.Errorf("list volume titles: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	out := make([]volumeRow, 0, 16)
	for rows.Next() {
		var r volumeRow
		if err := rows.Scan(&r.ID, &r.Book, &r.Title); err != nil {
			return nil, fmt.Errorf("scan volume title: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list volume titles: %w", err)
	}
	return out, nil
}

// planVolumeRenames 为整批卷规划改名：按书分组，组内按 (idx, id) 顺序。
func planVolumeRenames(rows []volumeRow) []volumeRename {
	out := make([]volumeRename, 0, 4)
	for start := 0; start < len(rows); {
		end := start
		for end < len(rows) && rows[end].Book == rows[start].Book {
			end++
		}
		out = append(out, planBookVolumeRenames(rows[start:end])...)
		start = end
	}
	return out
}

// planBookVolumeRenames 规划一本书的改名：每组同 (TRIM(title)) 保留首个，
// 其余改成「原标题 (2)」「原标题 (3)」…。
//
// 先占位全书「本来就唯一」的名字，再给重复项找空位：否则重名卷可能被改成
// 本书另一个真实存在的「原标题 (2)」，撞上紧接着要建的唯一索引。
func planBookVolumeRenames(rows []volumeRow) []volumeRename {
	reserved := make(map[string]struct{}, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if _, dup := seen[r.Title]; dup {
			continue
		}
		seen[r.Title] = struct{}{}
		reserved[r.Title] = struct{}{}
	}
	out := make([]volumeRename, 0, len(rows))
	kept := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if _, dup := kept[r.Title]; !dup {
			kept[r.Title] = struct{}{} // 首个保留原名
			continue
		}
		title := nextFreeVolumeTitle(r.Title, reserved)
		reserved[title] = struct{}{}
		out = append(out, volumeRename{ID: r.ID, Title: title})
	}
	return out
}

// nextFreeVolumeTitle 从「原标题 (2)」起找第一个本书还没占用的名字。
func nextFreeVolumeTitle(base string, reserved map[string]struct{}) string {
	for k := 2; ; k++ {
		candidate := fmt.Sprintf("%s (%d)", base, k)
		if _, taken := reserved[candidate]; !taken {
			return candidate
		}
	}
}

// applyVolumeRenames 逐条改标题（条数由调用方写进迁移日志）。
//
// 不动 books.updated_at：迁移只修数据形状，不改「这本书最后被改过」的业务时间。
func applyVolumeRenames(ctx context.Context, tx *sql.Tx, renames []volumeRename) error {
	for _, r := range renames {
		if _, err := tx.ExecContext(ctx,
			"UPDATE volumes SET title = ? WHERE id = ?", r.Title, r.ID); err != nil {
			return fmt.Errorf("rename volume %d: %w", r.ID, err)
		}
	}
	return nil
}
