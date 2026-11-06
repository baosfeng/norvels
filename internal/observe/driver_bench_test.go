package observe

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // 原生驱动，benchmark 的对照组
)

// benchDB 建一个只有 64 行的小库：查询成本落在「驱动调用 + 解析」上，包装开销才看得见。
func benchDB(tb testing.TB, driverName string) *sql.DB {
	tb.Helper()
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		tb.Fatalf("open %s: %v", driverName, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		tb.Fatalf("create: %v", err)
	}
	for i := 0; i < 64; i++ {
		if _, err := db.Exec("INSERT INTO t(id, v) VALUES (?, ?)", i, "value"); err != nil {
			tb.Fatalf("insert: %v", err)
		}
	}
	return db
}

func benchQuery(b *testing.B, driverName string) {
	ResetForTest()
	db := benchDB(b, driverName)
	defer db.Close() //nolint:errcheck // 基准结束后关闭，错误无关紧要
	ctx := context.Background()
	var out string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.QueryRowContext(ctx, "SELECT v FROM t WHERE id = ?", 7).Scan(&out); err != nil {
			b.Fatalf("query: %v", err)
		}
	}
}

func benchExec(b *testing.B, driverName string) {
	ResetForTest()
	db := benchDB(b, driverName)
	defer db.Close() //nolint:errcheck // 同上
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.ExecContext(ctx, "UPDATE t SET v = ? WHERE id = ?", "value", i%64); err != nil {
			b.Fatalf("exec: %v", err)
		}
	}
}

// 对照组：modernc 原生驱动（sql.Open("sqlite", ...)）。
func BenchmarkNativeQuery(b *testing.B) { benchQuery(b, "sqlite") }
func BenchmarkNativeExec(b *testing.B)  { benchExec(b, "sqlite") }

// 实验组：observe 的计时包装驱动。
func BenchmarkWrappedQuery(b *testing.B) { benchQuery(b, DriverName) }
func BenchmarkWrappedExec(b *testing.B)  { benchExec(b, DriverName) }
