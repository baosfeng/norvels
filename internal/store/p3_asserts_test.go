package store

import (
	"errors"
	"fmt"
	"testing"
)

// P3 测试的公共断言助手：把「比较 + 报错」收成一行。
//
// 为什么需要：门禁 4 的圈复杂度把 _test.go 也算在内，测试里堆十几个 if 照样超标。
// 断言收进助手之后，测试函数只剩「取数 → 断言」的直线代码，失败信息也更统一。

// requireEqual 断言 got == want（泛型覆盖字符串/数字/布尔）。
func requireEqual[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v，期望 %v", what, got, want)
	}
}

// requireOK 断言 err 为 nil（名字避开 append_test.go 里既有的 requireNoErr，参数顺序也不同）。
func requireOK(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", what, err)
	}
}

// requireErr 断言 err 命中目标哨兵（errors.Is）。
func requireErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s err = %v，期望 %v", what, err, want)
	}
}

// requireSliceEqual 断言字符串切片的内容与顺序一致。
func requireSliceEqual(t *testing.T, what string, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s = %v，期望 %v", what, got, want)
	}
}
