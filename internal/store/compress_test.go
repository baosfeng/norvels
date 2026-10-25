package store

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// 本文件锁死「按块压缩」的契约。两个层次分开测：
//   - compressContent/decompressContent：字节进、字节出，不许有跨块状态；
//   - splitBlocks/joinChapters/chapterAt：块边界与块内切片的正确性。

// TestCompressContentRoundTrip 覆盖契约要求的全部往返场景。
func TestCompressContentRoundTrip(t *testing.T) {
	long := []byte(strings.Repeat("长夜书简摊在灯下，青川剑砚压着纸角。窗外的风穿过廊下，铜铃一声接一声。", 3000))
	cases := []struct {
		name string
		in   []byte
	}{
		{"空块", nil},
		{"单个汉字", []byte("长")},
		{"超短块（低于压缩阈值）", []byte("短")},
		{"刚好到阈值", bytes.Repeat([]byte("a"), contentRawThreshold)},
		{"普通块", []byte("长夜书简摊在灯下，青川剑砚压着纸角。窗外有风穿过廊下，铜铃一声接一声。")},
		{"含 U+FFFD 替换字符", []byte("正文里有坏字节留下的 �� 替换符，还有正常汉字。")},
		{"私用区字符（反爬站常用）", []byte("这里有私用区字符：，往返不能变。")},
		{"纯 ASCII", bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 100)},
		{"十多万字长块", long},
		{"接近 1 MB 的块", bytes.Repeat([]byte(strings.Repeat("长夜", 100)), 2000)},
		{"含控制字符", []byte("第一行\n\t第二行\r\n第三行\x00\x01结束")},
		{"全空白", bytes.Repeat([]byte(" \n\t"), 200)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			packed, err := compressContent(c.in)
			if err != nil {
				t.Fatalf("compressContent: %v", err)
			}
			back, err := decompressContent(packed)
			if err != nil {
				t.Fatalf("decompressContent: %v", err)
			}
			if !bytes.Equal(back, c.in) {
				t.Errorf("往返不一致：原文 %d 字节，还原 %d 字节", len(c.in), len(back))
			}
		})
	}
}

// TestCompressContentEmptyIsNil 空块必须编码成 nil（SQL 里落 NULL / 零长 BLOB）。
func TestCompressContentEmptyIsNil(t *testing.T) {
	got, err := compressContent(nil)
	if err != nil {
		t.Fatalf("compressContent(nil): %v", err)
	}
	if got != nil {
		t.Errorf("空块应返回 nil，实际 %v（长度 %d）", got, len(got))
	}
	back, err := decompressContent(nil)
	if err != nil || len(back) != 0 {
		t.Errorf("decompressContent(nil) = %q, %v", back, err)
	}
}

// TestCompressContentAlgorithmMarker 校验首字节标记与「太小则退回原文」策略。
func TestCompressContentAlgorithmMarker(t *testing.T) {
	short := bytes.Repeat([]byte("a"), contentRawThreshold-1)
	got, err := compressContent(short)
	if err != nil {
		t.Fatalf("compressContent: %v", err)
	}
	if got[0] != contentAlgoRaw {
		t.Errorf("超短块首字节 = 0x%02x，期望 0x%02x（未压缩）", got[0], contentAlgoRaw)
	}
	if len(got) != len(short)+1 {
		t.Errorf("未压缩载荷长度 = %d，期望 %d", len(got), len(short)+1)
	}

	// 正常的 1 MB 级块必须打 zstd 标记并被显著压小。
	repetitive := []byte(strings.Repeat("长夜书简的青川剑砚。", 100000))
	big, err := compressContent(repetitive)
	if err != nil {
		t.Fatalf("compressContent: %v", err)
	}
	if big[0] != contentAlgoZstd {
		t.Errorf("正常块首字节 = 0x%02x，期望 0x%02x（zstd）", big[0], contentAlgoZstd)
	}
	if len(big) >= len(repetitive)/5 {
		t.Errorf("压缩后 %d 字节，原文 %d 字节——压缩未生效", len(big), len(repetitive))
	}
}

// TestCompressContentDeterministic 同一输入必须得到同一输出（体积对账依赖它）。
func TestCompressContentDeterministic(t *testing.T) {
	in := []byte(strings.Repeat("长夜书简的青川剑砚。", 5000))
	first, err := compressContent(in)
	if err != nil {
		t.Fatalf("compressContent: %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := compressContent(in)
		if err != nil {
			t.Fatalf("compressContent #%d: %v", i, err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("第 %d 次压缩结果与首次不同（非确定性）", i+1)
		}
	}
}

// TestDecompressContentIndependentPerBlock 是核心契约：
// 解压必须能按「块」为单位独立完成，不依赖其他块的解压状态。
func TestDecompressContentIndependentPerBlock(t *testing.T) {
	blocks := [][]byte{
		[]byte("第一块：长夜书简摊开，灯芯爆了一下。"),
		[]byte("第二块：青川剑砚压着纸角，墨迹慢慢洇开。"),
		nil,
		[]byte("第四块：荒渡的灯影在雾里晃。"),
	}
	packed := make([][]byte, len(blocks))
	for i, b := range blocks {
		p, err := compressContent(b)
		if err != nil {
			t.Fatalf("压缩第 %d 块: %v", i, err)
		}
		packed[i] = p
	}

	// 逆序 + 跳跃地单独解压：任何跨块状态都会在这里暴露。
	for _, i := range []int{3, 0, 2, 1} {
		got, err := decompressContent(packed[i])
		if err != nil {
			t.Fatalf("解压第 %d 块: %v", i, err)
		}
		if !bytes.Equal(got, blocks[i]) {
			t.Errorf("第 %d 块解压结果 = %q，期望 %q", i, got, blocks[i])
		}
	}
}

// TestDecompressContentFlateCompat 历史 0x01 标记（flate）必须仍可读。
//
// 格式位一旦写进磁盘就该永远读得回来，否则将来改算法就是一次静默数据丢失。
func TestDecompressContentFlateCompat(t *testing.T) {
	raw := []byte(strings.Repeat("长夜书简的历史块。", 200))
	packed, err := compressLegacyFlate(raw)
	if err != nil {
		t.Fatalf("构造 flate 块: %v", err)
	}
	if packed[0] != contentAlgoFlate {
		t.Fatalf("首字节 = 0x%02x，期望 flate 标记 0x%02x", packed[0], contentAlgoFlate)
	}
	back, err := decompressContent(packed)
	if err != nil {
		t.Fatalf("decompressContent(flate): %v", err)
	}
	if !bytes.Equal(back, raw) {
		t.Errorf("flate 块往返不一致：%d vs %d 字节", len(back), len(raw))
	}
}

// TestDecompressContentCorrupt 覆盖损坏/未知格式的错误分类。
func TestDecompressContentCorrupt(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"未知算法标记", []byte{0x7f, 'a', 'b'}},
		{"zstd 载荷被截断", []byte{contentAlgoZstd, 0x01, 0x02, 0x03}},
		{"flate 载荷被截断", []byte{contentAlgoFlate, 0x01, 0x02}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := decompressContent(c.in); !errors.Is(err, ErrCorruptContent) {
				t.Errorf("err = %v，期望 ErrCorruptContent", err)
			}
		})
	}
}

// TestChapterAtSlicesBlock 校验块内切片：正常区间、空章与越界。
func TestChapterAtSlicesBlock(t *testing.T) {
	block := []byte("第一章正文第二章正文第三章正文")

	t.Run("正常切片", func(t *testing.T) {
		got, err := chapterAt(block, 15, 15) // 「第二章正文」 的字节区间
		if err != nil {
			t.Fatalf("chapterAt: %v", err)
		}
		if got != "第二章正文" {
			t.Errorf("切片 = %q，期望 第二章正文", got)
		}
	})
	t.Run("零长度返回空串", func(t *testing.T) {
		got, err := chapterAt(block, 0, 0)
		if err != nil || got != "" {
			t.Errorf("chapterAt(len=0) = %q, %v", got, err)
		}
	})
	t.Run("越界必须报错而不是截断", func(t *testing.T) {
		if _, err := chapterAt(block, 0, len(block)+1); !errors.Is(err, ErrCorruptContent) {
			t.Errorf("越界错误 = %v，期望 ErrCorruptContent", err)
		}
		if _, err := chapterAt(block, -1, 2); !errors.Is(err, ErrCorruptContent) {
			t.Errorf("负偏移错误 = %v，期望 ErrCorruptContent", err)
		}
	})
}

// TestSplitBlocksByChapterBoundary 校验块按章节边界切、目标大小 1 MB。
func TestSplitBlocksByChapterBoundary(t *testing.T) {
	t.Run("空输入", func(t *testing.T) { checkSplitEmpty(t) })
	t.Run("总量不足一块", func(t *testing.T) { checkSplitSingleBlock(t) })
	t.Run("按 1 MB 切成多块", func(t *testing.T) { checkSplitMultiBlock(t) })
	t.Run("单章超过 1 MB 时独占一块", func(t *testing.T) { checkSplitOversizeChapter(t) })
	t.Run("块边界必须覆盖每一章且不重叠", func(t *testing.T) { checkSplitCoverage(t) })
}

// checkSplitEmpty 空输入返回 nil（调用方据此跳过建块）。
func checkSplitEmpty(t *testing.T) {
	t.Helper()
	if got := splitBlocks(nil); got != nil {
		t.Errorf("splitBlocks(nil) = %v", got)
	}
}

// checkSplitSingleBlock 总量不足 1 MB 时只有一块，覆盖全部章节。
func checkSplitSingleBlock(t *testing.T) {
	t.Helper()
	got := splitBlocks([]int{100, 200, 300})
	if len(got) != 1 || got[0] != [2]int{0, 2} {
		t.Errorf("splitBlocks = %v，期望单块覆盖全部章节", got)
	}
}

// checkSplitMultiBlock 超过 1 MB 时按章节边界切成多块。
func checkSplitMultiBlock(t *testing.T) {
	t.Helper()
	// 每章 300 KB，4 章合计 1.2 MB → 前 3 章一块（900 KB），第 4 章单独一块。
	sizes := []int{300 << 10, 300 << 10, 300 << 10, 300 << 10}
	got := splitBlocks(sizes)
	if len(got) != 2 {
		t.Fatalf("块数 = %d，期望 2：%v", len(got), got)
	}
	if got[0] != [2]int{0, 2} || got[1] != [2]int{3, 3} {
		t.Errorf("块边界 = %v，期望 [[0 2] [3 3]]", got)
	}
}

// checkSplitOversizeChapter 单章超过目标大小时独占一块。
func checkSplitOversizeChapter(t *testing.T) {
	t.Helper()
	got := splitBlocks([]int{100, 2 << 20, 100})
	if len(got) != 3 {
		t.Fatalf("块数 = %d，期望 3（超长章独占）：%v", len(got), got)
	}
	if got[1] != [2]int{1, 1} {
		t.Errorf("超长章块 = %v，期望单独一块", got[1])
	}
}

// checkSplitCoverage 块边界必须无缝覆盖每一章。
func checkSplitCoverage(t *testing.T) {
	t.Helper()
	sizes := make([]int, 50)
	for i := range sizes {
		sizes[i] = 100 << 10 // 每章 100 KB，50 章 → 5 MB
	}
	blocks := splitBlocks(sizes)
	covered, prevLast := 0, -1
	for _, span := range blocks {
		if span[0] != prevLast+1 {
			t.Fatalf("块出现空隙或重叠：前块末 %d，本块首 %d", prevLast, span[0])
		}
		if span[1] < span[0] {
			t.Fatalf("块区间非法: %v", span)
		}
		covered += span[1] - span[0] + 1
		prevLast = span[1]
	}
	if covered != len(sizes) || prevLast != len(sizes)-1 {
		t.Errorf("覆盖 %d 章，末章下标 %d，期望 %d / %d", covered, prevLast, len(sizes), len(sizes)-1)
	}
}

// TestJoinChaptersOffsets 校验拼接后的块内偏移与长度。
func TestJoinChaptersOffsets(t *testing.T) {
	contents := []string{"第一章正文", "", "第三章正文"}
	raw, offsets := joinChapters(contents)

	want := "第一章正文第三章正文"
	if string(raw) != want {
		t.Errorf("拼接结果 = %q，期望 %q", raw, want)
	}
	if len(offsets) != len(contents) {
		t.Fatalf("偏移数量 = %d，期望 %d", len(offsets), len(contents))
	}
	if offsets[0] != 0 {
		t.Errorf("首章偏移 = %d，期望 0", offsets[0])
	}
	// 第二章是空串，偏移应等于第一章末尾。
	firstLen := len(contents[0])
	if offsets[1] != firstLen {
		t.Errorf("空章偏移 = %d，期望 %d", offsets[1], firstLen)
	}
	if offsets[2] != firstLen {
		t.Errorf("第三章偏移 = %d，期望 %d（空章不占字节）", offsets[2], firstLen)
	}
	// 逐章切片必须还原原文。
	for i, c := range contents {
		got, err := chapterAt(raw, offsets[i], len(c))
		if err != nil {
			t.Fatalf("chapterAt(%d): %v", i, err)
		}
		if got != c {
			t.Errorf("第 %d 章切片 = %q，期望 %q", i, got, c)
		}
	}
}

// FuzzDecompressContent 保证任意字节输入都不会 panic（库里的旧数据可能损坏）。
func FuzzDecompressContent(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{contentAlgoRaw, 'a'})
	f.Add([]byte{contentAlgoFlate})
	f.Add([]byte{contentAlgoZstd})
	payload, _ := compressContent([]byte(strings.Repeat("长夜书简", 200)))
	f.Add(payload)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = decompressContent(b) // 不 panic 即可
	})
}
