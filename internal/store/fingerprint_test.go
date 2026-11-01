package store

import (
	"strings"
	"testing"
)

// 本文件锁死「章级指纹」的纯函数口径（设计 §4）：归一化抹掉什么、FNV-1a 怎么算、
// 空章与纯标点章、超长章分别是什么行为。判定链路（dedupe_test.go）只依赖这里的输出。

// TestNormalizeChapterText 归一化只抹排版差异：空白全删、全角/半角标点统一。
func TestNormalizeChapterText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"空串", "", ""},
		{"只有空白", " \t\r\n\u3000\u00a0", ""},
		{"删空白保留内容", "他 说：\n你\t好", "他说:你好"},
		{"中文标点折半角", "。，、；：？！", ".,,;:?!"},
		{"括号与书名号", "（括号）【方】《书名》「引」", "(括号)[方]<书名>\"引\""},
		{"破折号省略号", "——……", "--.."},
		{"全角 ASCII 折半角", "ＡＢＣ１２３！＂＃", "ABC123!\"#"},
		{"半角原样保留", "Hello, World!", "Hello,World!"},
		{"内容字符不折", "华夏國", "华夏國"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeChapterText(tc.in); got != tc.want {
				t.Errorf("NormalizeChapterText(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestChapterFingerprintMatchesNormalizedPath 流式实现必须与「先归一化再哈希」逐字一致：
// 这条对拍是允许流式优化的唯一依据，改任一实现都会红。
func TestChapterFingerprintMatchesNormalizedPath(t *testing.T) {
	contents := []string{
		"华夏国梅宁市，午后的秋日，微风拂过城楼。",
		"他站在城楼上，望着远方的梅宁城与风雪。\n\n第二段。",
		"。。。",
		"ＡＢＣ１２３",
		"  多  空  白  ",
	}
	for _, content := range contents {
		hash, bytes, ok := chapterFingerprint(content)
		if !ok {
			t.Fatalf("%q 不该被判成空章", content)
		}
		if want := ChapterHash(NormalizeChapterText(content)); hash != want {
			t.Errorf("%q 的流式哈希 %d 与归一化后哈希 %d 不一致", content, hash, want)
		}
		if bytes != int64(len(content)) {
			t.Errorf("%q 的字节权重 = %d，期望原始字节数 %d", content, bytes, len(content))
		}
	}
}

// TestChapterFingerprintIgnoresFormatting 只差排版（空白/全半角标点）的正文必须同指纹。
func TestChapterFingerprintIgnoresFormatting(t *testing.T) {
	base, _, ok := chapterFingerprint("华夏国梅宁市，午后的秋日。")
	if !ok {
		t.Fatal("基准正文不该是空章")
	}
	variants := []string{
		"华夏国梅宁市,午后的秋日.",
		"华夏国梅宁市，\n午后的秋日。",
		"  华夏国梅宁市 ，午后 的秋日。 ",
		"华夏国梅宁市，午后的秋日。\u3000",
	}
	for _, v := range variants {
		if hash, _, _ := chapterFingerprint(v); hash != base {
			t.Errorf("排版变体 %q 的指纹 %d 与基准 %d 不一致", v, hash, base)
		}
	}
	if hash, _, _ := chapterFingerprint("华夏国梅宁市，午后的冬日。"); hash == base {
		t.Error("改了一个字（秋→冬）的正文不该同指纹")
	}
}

// TestChapterFingerprintEmptyChapter 空章（含只有空白的章）不参与判定：ok=false，不带字节。
func TestChapterFingerprintEmptyChapter(t *testing.T) {
	for _, content := range []string{"", " ", "\n\n\t", "\u3000\u3000"} {
		hash, bytes, ok := chapterFingerprint(content)
		if ok || hash != 0 || bytes != 0 {
			t.Errorf("空章 %q 应返回 (0,0,false)，实际 (%d,%d,%v)", content, hash, bytes, ok)
		}
	}
}

// TestChapterFingerprintPurePunctuation 纯标点章：本身有内容（不丢），全角半角算同一章。
func TestChapterFingerprintPurePunctuation(t *testing.T) {
	hash, bytes, ok := chapterFingerprint("。。。！！")
	if !ok || hash == 0 || bytes != int64(len("。。。！！")) {
		t.Fatalf("纯标点章 = (%d,%d,%v)，期望非零指纹且保留字节权重", hash, bytes, ok)
	}
	if half, _, _ := chapterFingerprint("...!!"); half != hash {
		t.Errorf("全角纯标点与半角纯标点应同指纹：%d vs %d", hash, half)
	}
	if other, _, _ := chapterFingerprint("？？"); other == hash {
		t.Error("不同的纯标点章不该同指纹")
	}
}

// TestChapterFingerprintLongChapter 超长章（单章约 4 MB）：不 panic、可复现、排版不影响指纹。
func TestChapterFingerprintLongChapter(t *testing.T) {
	long := strings.Repeat("华夏国梅宁市，午后的秋日。", 100000)
	if len(long) < 3<<20 {
		t.Fatalf("超长章只有 %d 字节，用例强度不够", len(long))
	}
	hash, bytes, ok := chapterFingerprint(long)
	if !ok || hash == 0 || bytes != int64(len(long)) {
		t.Fatalf("超长章 = (%d,%d,%v)", hash, bytes, ok)
	}
	again, _, _ := chapterFingerprint(long)
	if again != hash {
		t.Errorf("同一段超长正文两次哈希不一致：%d vs %d", hash, again)
	}
	// 每句后面插一个换行：归一化后与原文相同，指纹必须一致。
	wrapped := strings.ReplaceAll(long, "。", "。\n")
	if h, _, _ := chapterFingerprint(wrapped); h != hash {
		t.Errorf("换行风格不同的超长正文指纹应相同：%d vs %d", h, hash)
	}
}

// BenchmarkChapterFingerprint 是流式指纹的单章吞吐基准（约 20 KB 一章）。
func BenchmarkChapterFingerprint(b *testing.B) {
	content := strings.Repeat("梅宁城的风从城楼吹过，少年按住剑柄，远处的雪线压得很低。\n", 600)
	b.SetBytes(int64(len(content)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := chapterFingerprint(content); !ok {
			b.Fatal("基准正文不该是空章")
		}
	}
}
