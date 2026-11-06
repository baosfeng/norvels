package store

import (
	"os"
	"testing"
)

// TestMain 在 store 包的单测里统一关掉后台任务。
//
// 后台 goroutine（正文对账刷新 / 指纹覆盖回填 / 健康计数回填）会活到进程结束，
// 而每个用例的库在 t.TempDir 里：goroutine 若在清理之后写一次，WAL/-shm 被重新造出来，
// TempDir RemoveAll 就报 "directory not empty"（随机命中某个用例，与用例本身无关）。
// 关闭后读取侧走实时 SQL，数字口径不变；需要回填的用例直接调 RebuildXxx 同步跑。
func TestMain(m *testing.M) {
	os.Setenv(EnvDisableBackground, "1")
	os.Exit(m.Run())
}
