package parser

import (
	"strings"
	"testing"
)

// TestCleanerDropsCopyrightAndWatermarkLines 版权声明行、以「声明：」开头的授权行、
// 混淆水印行一律整行丢弃——它们是 cleanContentAt 里三条「命中即丢」的分支：
// 超过 600 字节的版权串行不走整行噪声词判定，靠版权串兜住；短授权行靠 reCopyright；
// 标点密集的水印行靠 looksObfuscated（需 checkOb）。
func TestCleanerDropsCopyrightAndWatermarkLines(t *testing.T) {
	c := &cleaner{checkOb: true, stats: &cleanStats{}}
	longCopyright := strings.Repeat("本书仅供学习交流，", 80) + "请支持正版"
	cases := []struct {
		name string
		line string
	}{
		{"版权串长行（>600 字节，跳过整行噪声词判定）", longCopyright},
		{"声明开头的授权短行", "声明：本书由作者授权，禁止转载。"},
		{"标点密集的混淆水印行", "群:;;;;,.,.,!?:'"},
	}
	for _, tc := range cases {
		if got := c.cleanContent(tc.line); got != "" {
			t.Errorf("%s 应被整行丢弃，实际保留 %q", tc.name, got)
		}
	}
	if c.stats.raw != len(cases) {
		t.Errorf("丢弃计数：raw = %d，want %d", c.stats.raw, len(cases))
	}
	if c.stats.drop != len(cases) {
		t.Errorf("丢弃计数：drop = %d，want %d", c.stats.drop, len(cases))
	}
}

// TestCleanerKeepsNormalLines 正常正文不被上述几条丢弃分支误伤。
func TestCleanerKeepsNormalLines(t *testing.T) {
	c := &cleaner{checkOb: true, stats: &cleanStats{}}
	const text = "第一章 起风\n\n梅宁城的风从城楼吹过，少年按住剑柄。"
	if got := c.cleanContent(text); !strings.Contains(got, "梅宁城的风从城楼吹过") {
		t.Fatalf("正常正文被误删：%q", got)
	}
	if c.stats.drop != 0 {
		t.Errorf("正常正文不该记丢弃，drop = %d", c.stats.drop)
	}
}
