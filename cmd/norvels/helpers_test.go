package main

import "testing"

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"零字节", 0, "0 B"},
		{"小于 1KB 用字节", 1023, "1023 B"},
		{"恰好 1KB", 1024, "1.00 KB"},
		{"1.5KB", 1536, "1.50 KB"},
		{"1MB", 1024 * 1024, "1.00 MB"},
		{"1GB", 1024 * 1024 * 1024, "1.00 GB"},
		{"1TB", 1024 * 1024 * 1024 * 1024, "1.00 TB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanBytes(tc.in); got != tc.want {
				t.Fatalf("humanBytes(%d) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTrim(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"短于上限原样返回", "短文本", 10, "短文本"},
		{"恰好等于上限原样返回", "abcde", 5, "abcde"},
		{"超过上限截断并加省略号", "abcdef", 3, "abc…"},
		{"按 rune 而非字节截断", "中文标题很长", 2, "中文…"},
		{"上限为 0 时只剩省略号", "abc", 0, "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trim(tc.in, tc.n); got != tc.want {
				t.Fatalf("trim(%q, %d) = %q，期望 %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
