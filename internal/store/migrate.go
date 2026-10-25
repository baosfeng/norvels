package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// migrationsFS 打包全部编号迁移。
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsDir 是 embed 内的迁移目录。
const migrationsDir = "migrations"

// legacyBaselineVersion 是老库（跑整份 schema.sql 的时代）的基线：PRAGMA user_version=1。
const legacyBaselineVersion = 1

const migrationTable = "schema_migrations" // 版本记录表名。

type migration struct { // 一条待应用的迁移。
	Version int    // 文件名里的版本号（0001 → 1）
	Name    string // 完整文件名，例如 0001_init.sql
	SQL     string // 迁移 SQL 全文
	// Steps 是 SQL 之后要在同一事务内执行的 Go 步骤（可空）。
	//
	// 为什么需要它：SQLite 没有条件 DDL。像「表存在时才去删列」这种语句，
	// 在 chapters 表还不存在的畸形老库升级路径上会直接失败（0004 注释里记过同一个坑）。
	// 把这些步骤放进**同一个迁移事务**，既保证它们能先探测再执行，
	// 又守住「一条迁移一个事务、失败整体回滚」——不会出现 SQL 已提交、
	// 版本号已前进、而结构没改完的半迁移库。
	Steps []migrationStep
}

// migrationStep 是迁移 SQL 之后、同一事务内的一个 Go 步骤。
type migrationStep struct {
	Name string // 日志与报错里的人类可读名
	Run  func(ctx context.Context, tx *sql.Tx) error
}

// Label 是给日志与 CLI 用的人类可读描述。
func (m migration) Label() string { return fmt.Sprintf("%04d %s", m.Version, m.Name) }

// migrationStatus 描述一次迁移的结果，供启动日志与 CLI 打印。
type migrationStatus struct {
	FromVersion   int         // 迁移前库版本
	ToVersion     int         // 迁移后库版本
	Applied       []migration // 本次实际应用的迁移（升序）
	Total         int         // 打包的迁移总数
	AlreadyAtMax  bool        // 迁移前已是打包的最高版本
	AdoptedLegacy bool        // 识别到老库并直接补记 0001（未重跑建表）
	FailedVersion int         // 失败发生在哪个版本（0 = 未失败）
}

// versionInfo 是不落盘地探测出的库版本状态。
type versionInfo struct {
	Version  int  // 已应用的最高版本（老库按 1 计）
	HasTable bool // schema_migrations 表是否已存在
	Legacy   bool // 无版本记录但业务表已存在（旧版 Migrate 建的库）
}

// Migrate 把库升级到打包的最高版本。
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.MigrateTo(ctx, LatestVersion())
	return err
}

// MigrateTo 按版本升序应用「大于当前版本且不大于 target」的迁移，每条迁移一个事务。
//
// 任一迁移失败只回滚它自身并立即返回错误（调用方应让启动失败）；
// 已成功的前序迁移保持已应用，不回退。
func (s *Store) MigrateTo(ctx context.Context, target int) (migrationStatus, error) {
	all, err := loadMigrations()
	if err != nil {
		return migrationStatus{}, err
	}
	st := migrationStatus{Total: len(all)}
	if len(all) == 0 {
		return st, nil
	}
	latest := all[len(all)-1].Version
	if target <= 0 || target > latest {
		target = latest
	}
	// 版本守卫：在动任何 DDL 之前只读比较。必须在 ensureMigrationTable 之前——
	// 拒绝路径（库由更新版本迁移过）上不能往磁盘写一个字节，否则用户连
	// 「这个库原本是什么版本」都失去了判断依据，也谈不上回滚取证。
	if err := s.ensureSchemaNotNewer(ctx, latest); err != nil {
		return st, err
	}
	if err := s.ensureMigrationTable(ctx); err != nil {
		return st, err
	}
	if err := s.prepareBaseline(ctx, &st); err != nil {
		return st, err
	}
	cur, applyErr := s.applyRange(ctx, &st, all, target)
	st.ToVersion = cur
	if applyErr != nil {
		return st, applyErr
	}
	st.AlreadyAtMax = st.FromVersion >= latest
	// 性能索引不进迁移文件（老库升级路径上 chapters 可能还不存在），迁移成功后幂等补齐。
	if err := s.ensureCompositionIndex(ctx); err != nil {
		return st, err
	}
	return st, nil
}

// prepareBaseline 探测当前版本，并在识别到老库时补记 0001（不重跑建表）。
func (s *Store) prepareBaseline(ctx context.Context, st *migrationStatus) error {
	info, err := s.inspectVersion(ctx)
	if err != nil {
		return err
	}
	if info.Legacy {
		if err := s.recordVersion(ctx, legacyBaselineVersion); err != nil {
			return err
		}
		st.AdoptedLegacy = true
	}
	st.FromVersion = info.Version
	return nil
}

// applyRange 按序应用 (cur, target] 区间的迁移，返回「库实际停在的版本」。
//
// 失败时返回已应用到的版本与错误：调用方据此如实报告「停在哪个版本」。
func (s *Store) applyRange(ctx context.Context, st *migrationStatus, all []migration, target int) (int, error) {
	cur := st.FromVersion
	for _, m := range all {
		if m.Version <= cur || m.Version > target {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			st.FailedVersion = m.Version
			return cur, err
		}
		st.Applied = append(st.Applied, m)
		cur = m.Version
	}
	return cur, nil
}

// LatestVersion 返回打包的最高迁移版本（无迁移时为 0）。
func LatestVersion() int {
	all, err := loadMigrations()
	if err != nil || len(all) == 0 {
		return 0
	}
	return all[len(all)-1].Version
}

// PendingMigrations 只读地返回「大于库当前版本」的迁移，供 --dry-run 使用。
// 不建表、不写记录、不执行任何 DDL。
func (s *Store) PendingMigrations(ctx context.Context) ([]migration, versionInfo, error) {
	all, err := loadMigrations()
	if err != nil {
		return nil, versionInfo{}, err
	}
	info, err := s.inspectVersion(ctx)
	if err != nil {
		return nil, versionInfo{}, err
	}
	out := make([]migration, 0, len(all))
	for _, m := range all {
		if m.Version > info.Version {
			out = append(out, m)
		}
	}
	return out, info, nil
}

// ensureMigrationTable 建版本记录表（幂等，独立于任何编号迁移）。
func (s *Store) ensureMigrationTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS "+migrationTable+" ("+
			"version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)")
	if err != nil {
		return fmt.Errorf("store: create %s: %w", migrationTable, err)
	}
	return nil
}

// inspectVersion 探测库版本，不修改数据库。
//
// 判定顺序：schema_migrations 有记录 → 用它；没有记录但 books 表已存在 → 视为老库（版本 1）；
// 否则视为空库（版本 0）。
func (s *Store) inspectVersion(ctx context.Context) (versionInfo, error) {
	var info versionInfo
	hasTable, err := s.tableExists(ctx, migrationTable)
	if err != nil {
		return info, err
	}
	info.HasTable = hasTable
	if hasTable {
		v, err := s.maxRecordedVersion(ctx)
		if err != nil {
			return info, err
		}
		if v > 0 {
			info.Version = v
			return info, nil
		}
	}
	hasBooks, err := s.tableExists(ctx, "books")
	if err != nil {
		return info, err
	}
	if hasBooks {
		info.Version = legacyBaselineVersion
		info.Legacy = info.Version > 0
	}
	return info, nil
}

// maxRecordedVersion 返回 schema_migrations 里的最大版本（无记录为 0）。
func (s *Store) maxRecordedVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		"SELECT MAX(version) FROM "+migrationTable).Scan(&v); err != nil {
		return 0, fmt.Errorf("store: read schema version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// tableExists 判断 sqlite_master 中是否存在指定表/视图/虚拟表。
func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name = ?", name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check table %s: %w", name, err)
	}
	return n > 0, nil
}

// recordVersion 写入一条版本记录（已存在则忽略）。
func (s *Store) recordVersion(ctx context.Context, version int) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO "+migrationTable+"(version, applied_at) VALUES (?, ?)",
		version, formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("store: record migration %d: %w", version, err)
	}
	return nil
}

// applyMigration 在单个事务内执行迁移 SQL 并记录版本；任一步失败整体回滚。
func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration %d: %w", m.Version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := runMigrationSteps(ctx, tx, m); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration %d: %w", m.Version, err)
	}
	committed = true
	return nil
}

// runMigrationSteps 在同一个事务内依次执行：迁移 SQL → Go 步骤 → 写版本记录。
//
// 任一步失败都返回错误，由 applyMigration 整体回滚：版本记录与已执行的结构变更
// 一起消失，库停在迁移前状态。
func runMigrationSteps(ctx context.Context, tx *sql.Tx, m migration) error {
	if strings.TrimSpace(m.SQL) == "" {
		return fmt.Errorf("store: migration %s is empty", m.Name)
	}
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("store: migration %s failed: %w", m.Name, err)
	}
	for _, step := range m.Steps {
		if err := step.Run(ctx, tx); err != nil {
			return fmt.Errorf("store: migration %s step %q failed: %w", m.Name, step.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+migrationTable+"(version, applied_at) VALUES (?, ?)",
		m.Version, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: record migration %d: %w", m.Version, err)
	}
	return nil
}

// loadMigrations 读取 embed 内的迁移文件，按版本升序返回。
//
// 命名约束 NNNN_name.sql；版本号重复、文件名不合规、目录为空都直接报错，
// 避免静默跳过迁移。编号只增不补：新增迁移一律追加更大的版本号。
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("store: read migrations: %w", err)
	}
	out := make([]migration, 0, len(entries))
	seen := make(map[int]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseMigrationVersion(e.Name())
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("store: duplicate migration version %d: %s / %s", version, prev, e.Name())
		}
		seen[version] = e.Name()
		data, err := fs.ReadFile(migrationsFS, migrationsDir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("store: read migration %s: %w", e.Name(), err)
		}
		out = append(out, migration{
			Version: version,
			Name:    e.Name(),
			SQL:     string(data),
			Steps:   migrationStepsByVersion[version],
		})
	}
	if len(out) == 0 {
		return nil, errors.New("store: no migrations embedded")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// ---------- 供 cmd 包使用的导出别名 ----------
//
// 迁移的内部类型保持小写（包内自用），cmd/norvels 只需要读取结果摘要与版本信息，
// 因此额外导出只读别名，避免把内部字段暴露成可变 API。

// VersionInfo 是库版本探测结果（cmd 的 --dry-run 输出需要）。
type VersionInfo = versionInfo

// MigrationStatus 是一次迁移的结果摘要（cmd 的 migrate 输出需要）。
type MigrationStatus = migrationStatus

// parseMigrationVersion 解析文件名前缀 "0001" → 1。
func parseMigrationVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	num, _, ok := strings.Cut(base, "_")
	if !ok || len(num) != 4 {
		return 0, fmt.Errorf("store: bad migration filename %q (want NNNN_name.sql)", name)
	}
	v, err := strconv.Atoi(num)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("store: bad migration version in %q", name)
	}
	return v, nil
}
