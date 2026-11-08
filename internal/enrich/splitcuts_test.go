package enrich

import (
	"strings"
	"testing"
)

// TestSplitCutsNotEnoughWords 覆盖「字数不够就不切」。
func TestSplitCutsNotEnoughWords(t *testing.T) {
	if cuts := SplitCuts(strings.Repeat("字", 100), 50, 200); cuts != nil {
		t.Fatalf("总字数不足 minWords 不该切，实际 %v", cuts)
	}
}

// TestSplitCutsBadParams 覆盖参数非法：目标不小于阈值等于「不切」。
func TestSplitCutsBadParams(t *testing.T) {
	body := strings.Repeat("字\n", 500)
	if cuts := SplitCuts(body, 8000, 8000); cuts != nil {
		t.Fatalf("targetWords >= minWords 应返回 nil，实际 %v", cuts)
	}
	if cuts := SplitCuts(body, 0, 100); cuts != nil {
		t.Fatalf("targetWords=0 应返回 nil，实际 %v", cuts)
	}
}

// TestSplitCutsNormal 覆盖正常切分：每段大约目标字数、切点严格递增、不止一刀。
func TestSplitCutsNormal(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString(strings.Repeat("正", 200))
		b.WriteByte('\n')
	}
	body := b.String() // 8000 字 / 40 段
	cuts := SplitCuts(body, 2000, 5000)
	if len(cuts) < 2 {
		t.Fatalf("8000 字按 2000 目标应切出至少 2 刀，实际 %v", cuts)
	}
	prev := -1
	for _, c := range cuts {
		if c <= prev {
			t.Fatalf("切点必须严格递增，实际 %v", cuts)
		}
		prev = c
	}
}

// TestSplitCutsShortTailMerged 覆盖「尾巴太短的最后一刀被撤销」。
func TestSplitCutsShortTailMerged(t *testing.T) {
	var b strings.Builder
	b.WriteString(strings.Repeat("长", 3000))
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("尾", 100))
	b.WriteByte('\n')
	cuts := SplitCuts(b.String(), 1000, 2000)
	if len(cuts) != 0 {
		t.Fatalf("尾段只有 100 字，最后一刀应被撤销，实际 %v", cuts)
	}
}

// TestSplitCutsSingleLine 覆盖整篇没有换行：没有段落边界就不切（交给人处理）。
func TestSplitCutsSingleLine(t *testing.T) {
	if cuts := SplitCuts(strings.Repeat("字", 40000), 8000, 30000); cuts != nil {
		t.Fatalf("无段落边界不该硬切，实际 %v", cuts)
	}
}

// TestSplitCutsMaxPieces 覆盖切段上限 99。
func TestSplitCutsMaxPieces(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString(strings.Repeat("段", 100))
		b.WriteByte('\n')
	}
	cuts := SplitCuts(b.String(), 100, 200)
	if len(cuts) >= maxSplitPieces {
		t.Fatalf("切点数应少于 %d，实际 %d", maxSplitPieces, len(cuts))
	}
	if len(cuts) == 0 {
		t.Fatal("40 万字按 100 字目标应至少切一刀")
	}
}

// TestCountWords 覆盖字数口径：CJK 逐字、连续英文词计 1。
func TestCountWords(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"中文", 2},
		{"hello world", 2},
		{"中文 abc 中文", 5}, // 中文 2 + "abc" 一词 1 + 中文 2
		{"，。！", 0},
	}
	for _, c := range cases {
		if got := countWords(c.in); got != c.want {
			t.Errorf("countWords(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

// TestSplitCutsSentenceFallback 覆盖无段落边界但有句末标点时退化按句子切。
func TestSplitCutsSentenceFallback(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString(strings.Repeat("句", 200))
		b.WriteString("。")
	}
	body := b.String()
	cuts := SplitCuts(body, 2000, 5000)
	if len(cuts) < 2 {
		t.Fatalf("8000 字按句子应切出至少 2 刀，实际 %v", cuts)
	}
	prev := -1
	for _, c := range cuts {
		if c <= prev {
			t.Fatalf("切点必须严格递增，实际 %v", cuts)
		}
		prev = c
	}
}

// TestSplitCutsSentenceShortTailMerged 覆盖句子退化下的尾段过短撤销。
func TestSplitCutsSentenceShortTailMerged(t *testing.T) {
	var b strings.Builder
	b.WriteString(strings.Repeat("长", 3000))
	b.WriteString("。")
	b.WriteString(strings.Repeat("尾", 100))
	b.WriteString("。")
	cuts := SplitCuts(b.String(), 1000, 2000)
	if len(cuts) != 0 {
		t.Fatalf("尾段只有 100 字，最后一刀应被撤销，实际 %v", cuts)
	}
}

// TestSplitCutsSentenceMaxPieces 覆盖句子退化下的 99 段上限。
func TestSplitCutsSentenceMaxPieces(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString(strings.Repeat("句", 100))
		b.WriteString("。")
	}
	cuts := SplitCuts(b.String(), 100, 200)
	if len(cuts) >= maxSplitPieces {
		t.Fatalf("切点数应少于 %d，实际 %d", maxSplitPieces, len(cuts))
	}
	if len(cuts) == 0 {
		t.Fatal("40 万字按 100 字目标应至少切一刀")
	}
}
