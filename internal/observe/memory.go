package observe

import (
	"os"
	"strconv"
	"strings"
)

// procStatusPath 是 Linux 的进程状态文件；变量而非常量是为了让测试能喂假数据
// （macOS 上没有这个文件，CI 的 darwin 跑单测时读到空值即视为「拿不到」）。
var procStatusPath = "/proc/self/status"

// RSSMB 返回本进程的常驻内存（MB，取自 /proc/self/status 的 VmRSS）。
//
// 为什么不用 runtime.MemStats：本项目真正的内存大头是 SQLite 页缓存，
// 它走 libc.Xmalloc 落在 Go 堆之外，MemStats 完全看不见（实测 HeapAlloc 3.84 MB
// 而进程 RSS 470 MB）。要回答「这个进程现在占多少内存」，只能问内核。
//
// 拿不到（非 Linux、文件不可读、字段缺失）时返回 (0, false)，调用方按「无数据」处理，
// 不要当成「占了 0 字节」。
func RSSMB() (float64, bool) {
	b, err := os.ReadFile(procStatusPath)
	if err != nil {
		return 0, false
	}
	return parseRSSMB(string(b))
}

// parseRSSMB 从 /proc/self/status 的文本里取 VmRSS 并换算成 MB。
func parseRSSMB(status string) (float64, bool) {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || kb < 0 {
			return 0, false
		}
		return float64(kb) / 1024, true
	}
	return 0, false
}
