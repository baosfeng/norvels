package observe

import "testing"

// VmRSS 是排查内存时唯一可信的数字（Go 的 HeapAlloc 看不见 off-heap 的页缓存），
// 解析必须容忍字段缺失与脏数据，而不是把「读不到」当成「占 0 字节」。
func TestParseRSSMB(t *testing.T) {
	got, ok := parseRSSMB("Name:\tnorvels\nVmRSS:\t  470296 kB\nThreads:\t12\n")
	if !ok {
		t.Fatal("VmRSS 存在时应解析成功")
	}
	if got < 459 || got > 460 {
		t.Fatalf("470296 kB 应约等于 459.3 MB，实际 %.2f", got)
	}
}

func TestParseRSSMBMissing(t *testing.T) {
	for _, status := range []string{"", "Name:\tnorvels\n", "VmRSS:\n", "VmRSS:\tabc kB\n", "VmRSS:\t-1 kB\n"} {
		if _, ok := parseRSSMB(status); ok {
			t.Fatalf("输入 %q 不应解析成功", status)
		}
	}
}

// 读不到 /proc 时必须明确报「无数据」：调用方据此不打印误导性的 0 MB。
func TestRSSMBUnavailable(t *testing.T) {
	orig := procStatusPath
	procStatusPath = t.TempDir() + "/missing"
	t.Cleanup(func() { procStatusPath = orig })
	if _, ok := RSSMB(); ok {
		t.Fatal("文件不存在时应返回 ok=false")
	}
}
