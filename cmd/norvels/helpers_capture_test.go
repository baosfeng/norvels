package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout 把 os.Stdout 临时重定向到管道，返回函数执行期间写入的内容。
//
// printContentCheck 直接写 os.Stdout（CLI 的实际行为），要断言输出就得换个管道。
// 注意：必须在 goroutine 里读，否则管道缓冲写满会死锁。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(r)
		done <- string(buf)
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("关闭管道: %v", err)
	}
	os.Stdout = old
	out := <-done
	_ = r.Close()
	return out
}

// containsStr 是测试内的子串判断（避免为几处断言引入 strings 包的最小心智负担）。
func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
