package importer

import (
	"context"
	"sync"
)

// 本文件实现「取消运行中的导入任务」，是 spec §2.3 的 importer.Cancel 语义。
//
// 登记点在调用方（api 的 runImport / runUploadImport），而不是 Run/RunUpload 内部：
// 要取消的是调用方用 context.WithCancel 建的那个 ctx，cancel 函数只存在于
// 调用方手里——Run 拿得到 ctx，却拿不到它的 cancel。因此由建 ctx 的一方登记，
// 本包只负责「按任务 id 找到 cancel 并调用」。
//
// 一个进程同一时刻只跑一个导入任务（api 用 importMu + 库内 running 判重），
// 所以这里用「任务 id → cancel」的小 map 就够，不需要队列或并发多任务支持。

var (
	registryMu sync.Mutex
	registry   = make(map[int64]context.CancelFunc)
)

// Track 登记一个可取消的导入任务，返回注销函数（调用方 defer 执行）。
//
// 必须在启动导入 goroutine **之前**调用：先把取消入口挂好再开跑，
// 否则「库里已是 running、但还没登记」的那一瞬间到来的取消请求会被判成
// 「不可取消」，而对用户来说这就是一个能取消的任务。
//
// jobID <= 0 或 cancel 为 nil 时是空操作，返回的注销函数同样可安全调用。
func Track(jobID int64, cancel context.CancelFunc) func() {
	if jobID <= 0 || cancel == nil {
		return func() {}
	}
	registryMu.Lock()
	registry[jobID] = cancel
	registryMu.Unlock()
	return func() {
		registryMu.Lock()
		delete(registry, jobID)
		registryMu.Unlock()
	}
}

// Cancel 取消运行中的导入任务，返回是否命中。
//
// 未命中（false）= 这个 id 不在本进程运行：任务已结束，或任务记录是上一次进程
// 留下的僵尸，或 id 根本不存在。这三种情况都不写库——把「是否存在」交给
// store.GetImportJob 判定，本函数只回答「能不能取消」。
//
// 命中时也只发出取消信号：任务记录由导入 goroutine 收尾时写成 canceled，
// 这里不写库，避免两条写路径同时改同一行任务记录。
func Cancel(jobID int64) bool {
	if jobID <= 0 {
		return false
	}
	registryMu.Lock()
	cancel, ok := registry[jobID]
	registryMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}
