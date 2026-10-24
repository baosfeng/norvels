package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"norvels/internal/store"
)

func cmdMigrate(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	dryRun := fs.Bool("dry-run", false, "只打印将要应用的迁移，不修改数据库")
	toVersion := fs.Int("to", 0, "只迁移到指定版本（默认最新）")
	fs.Parse(args)

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if *dryRun {
		return runMigrateDryRun(ctx, st, *dbPath)
	}
	target := *toVersion
	if target <= 0 {
		target = store.LatestVersion()
	}
	status, err := st.MigrateTo(ctx, target)
	if err != nil {
		// 版本守卫拒绝时直接透出原文（见 main.go 同处注释）。
		if errors.Is(err, store.ErrSchemaTooNew) {
			return err
		}
		return fmt.Errorf("迁移失败（迁移 %d 出错，库版本停在 %d，未前进）: %w",
			status.FailedVersion, status.ToVersion, err)
	}
	printMigrationStatus(status, *dbPath)
	return nil
}

// runMigrateDryRun 打印待应用迁移，不建表也不写版本。
//
// 版本过新的库仍是只读诊断：先把「库版本 / 二进制支持版本」两个数摆出来，
// 再按守卫口径报错退出。不能只说「待应用 0 个，已是最新版本」——那句话在
// 库比二进制新的时候是错的，会把用户引向「没问题，直接启动」。
func runMigrateDryRun(ctx context.Context, st *store.Store, dbPath string) error {
	pending, info, err := st.PendingMigrations(ctx)
	if err != nil {
		return err
	}
	if guardErr := st.EnsureSchemaNotNewer(ctx); guardErr != nil {
		fmt.Fprintf(os.Stdout, "数据库: %s\n当前版本: %d\n当前二进制支持到: %d\n",
			dbPath, info.Version, store.LatestVersion())
		return guardErr
	}
	fmt.Fprintf(os.Stdout, "数据库: %s\n当前版本: %d%s\n待应用: %d 个\n",
		dbPath, info.Version, legacyNote(info), len(pending))
	for _, m := range pending {
		fmt.Fprintf(os.Stdout, "  %s\n", m.Label())
	}
	if len(pending) == 0 {
		fmt.Fprintln(os.Stdout, "（已是最新版本，无需迁移）")
	}
	return nil
}

// legacyNote 在 dry-run 输出里标注「这是旧版库」。
func legacyNote(info store.VersionInfo) string {
	if info.Legacy {
		return "（识别为旧版数据库：业务表已存在，将直接记为版本 1）"
	}
	return ""
}

// printMigrationStatus 打印迁移结果摘要。
func printMigrationStatus(status store.MigrationStatus, dbPath string) {
	fmt.Fprintf(os.Stdout, "数据库: %s\n", dbPath)
	if status.AdoptedLegacy {
		fmt.Fprintln(os.Stdout, "识别到旧版数据库（无 schema_migrations），已把版本 1 记为已应用，未重跑建表")
	}
	fmt.Fprintf(os.Stdout, "库版本 %d → 应用 %d 个迁移 → 版本 %d（打包共 %d 个）\n",
		status.FromVersion, len(status.Applied), status.ToVersion, status.Total)
	for _, m := range status.Applied {
		fmt.Fprintf(os.Stdout, "  已应用 %s\n", m.Label())
	}
	if len(status.Applied) == 0 {
		fmt.Fprintln(os.Stdout, "（无迁移可应用）")
	}
}

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGT"[exp])
}
