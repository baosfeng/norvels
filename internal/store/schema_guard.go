package store

import (
	"context"
	"fmt"
)

// 守卫要拦的情形：磁盘上的库由更新版本的二进制迁移过（库记录的最高迁移版本 >
// 本二进制打包的最高迁移版本）。这类库必须拒绝打开并写入——旧二进制不认识新结构，
// 会按自己那套更早的 schema 继续读写，还可能把版本号写回，造成**静默的数据损坏**。
// 对应哨兵 ErrSchemaTooNew 定义在 errors.go（与其他包级错误放一起）。

// SchemaVersionError 是「库版本过新」的具体原因，供调用方 errors.As 取证。
//
// 只在**只读检查**路径产生：构造它和返回它的过程都不写库。
type SchemaVersionError struct {
	DatabaseVersion int // 库里记录的最高迁移版本（schema_migrations）
	Supported       int // 当前二进制打包的最高迁移版本
}

// Error 返回给用户看的中文提示：说明现象、双方版本，以及两条可行出路。
func (e *SchemaVersionError) Error() string {
	return fmt.Sprintf(
		"数据库由更新版本（v%d）创建，当前二进制只支持到 v%d。请升级镜像，或从备份恢复（迁移只前进，不支持降级）",
		e.DatabaseVersion, e.Supported)
}

// Is 让 errors.Is(err, ErrSchemaTooNew) 成立，便于上层按哨兵分类错误。
func (e *SchemaVersionError) Is(target error) bool { return target == ErrSchemaTooNew }

// EnsureSchemaNotNewer 只读地校验「库版本 ≤ 本二进制支持的最高版本」。
//
// 正常路径返回 nil；库版本更高时返回 *SchemaVersionError（errors.Is 可判 ErrSchemaTooNew）。
// 本函数不建表、不写版本记录、不改 user_version：拒绝启动时必须让磁盘上的库保持原样，
// 否则用户连「从备份恢复」都失去了判断依据。
func (s *Store) EnsureSchemaNotNewer(ctx context.Context) error {
	return s.ensureSchemaNotNewer(ctx, LatestVersion())
}

// ensureSchemaNotNewer 是 EnsureSchemaNotNewer 的内部形态，允许调用方传入已解析的
// 最高版本，避免在一次迁移里重复解析 embed 目录。
func (s *Store) ensureSchemaNotNewer(ctx context.Context, supported int) error {
	info, err := s.inspectVersion(ctx)
	if err != nil {
		// 读不出版本：文件损坏或根本不是 SQLite 库。与「版本过新」明确区分，
		// 否则用户会照着「请升级镜像」的提示白折腾。
		return fmt.Errorf("store: 无法读取现有数据库的 schema 版本（文件可能已损坏或不是 SQLite 数据库）: %w", err)
	}
	return checkSchemaVersion(info.Version, supported)
}

// checkSchemaVersion 比较库版本与二进制支持的最高版本。
//
// 三种情形：库版本更高 → 拒绝；相等 → 放行（无需迁移）；更低 → 放行（走正常迁移）。
// 未初始化（0）的库也走放行：那是新建库，由迁移把版本建起来。
func checkSchemaVersion(dbVersion, supported int) error {
	if dbVersion > supported {
		return &SchemaVersionError{DatabaseVersion: dbVersion, Supported: supported}
	}
	return nil
}
