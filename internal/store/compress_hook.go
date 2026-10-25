package store

import "sync/atomic"

// compressFailureHook 是「注入块压缩失败」的测试开关（生产路径恒为 false）。
//
// 为什么用开关而不是靠篡改数据：0007 迁移唯一的失败点就是「压缩某一块」，
// 只有精确控制这一处，才能验证「已搬进块的书也会一起回滚」。
// 原子变量默认 false，生产读这条分支的成本可忽略。
var compressFailureHook atomic.Bool

// stubCompressFailure 打开失败钩子并注册恢复（供测试使用）。
func stubCompressFailure(t interface{ Cleanup(func()) }) func() {
	compressFailureHook.Store(true)
	restore := func() { compressFailureHook.Store(false) }
	t.Cleanup(restore)
	return restore
}
