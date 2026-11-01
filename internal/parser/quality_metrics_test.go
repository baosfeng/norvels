package parser

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"

	"norvels/internal/model"
)

// ---------- 合成语料（全部虚构，不含任何真实书名与正文） ----------

// syntheticNovel 拼一本合成书：chapters 章，每章 paragraphs 段、每段约 40 字。
// 段落里带章号段号，保证各段内容互不相同（正常正文不该被判成重复）。
func syntheticNovel(chapters, paragraphs int) string {
	var b strings.Builder
	for i := 1; i <= chapters; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "章 合成标题\n")
		for j := 0; j < paragraphs; j++ {
			b.WriteString("这是用于测试的合成正文段落，内容没有实际意义，只保证长度与标点接近中文小说，编号")
			b.WriteString(strconv.Itoa(i) + "-" + strconv.Itoa(j) + "。\n")
		}
	}
	return b.String()
}

// noisyNovel 在 syntheticNovel 的基础上，每章插入 4 行噪声（群号 / 网址 / 分隔线 / 广告）。
func noisyNovel(chapters, paragraphs int) string {
	var b strings.Builder
	for i := 1; i <= chapters; i++ {
		b.WriteString("第" + strconv.Itoa(i) + "章 合成标题\n")
		b.WriteString("更多小说资源尽在强鹿小说群：12345678\n")
		b.WriteString("www.example-shop.test\n")
		b.WriteString("===============\n")
		b.WriteString("QQ群：98765432\n")
		for j := 0; j < paragraphs; j++ {
			b.WriteString("这是用于测试的合成正文段落，内容没有实际意义，只保证长度与标点接近中文小说，编号")
			b.WriteString(strconv.Itoa(i) + "-" + strconv.Itoa(j) + "。\n")
		}
	}
	return b.String()
}

// parseSynthetic 用默认配置解析一段合成文本，失败即终止用例。
func parseSynthetic(t *testing.T, text string) *model.ParsedBook {
	t.Helper()
	book, err := New(DefaultOptions()).ParseBytes("合成书名.txt", []byte(text))
	if err != nil {
		t.Fatalf("解析合成文本失败: %v", err)
	}
	return book
}

// ---------- 正常中文正文 ----------

// TestQualityNormalText 覆盖「干净正文」：乱码 0、噪声 0、结构满分、无重复段落。
func TestQualityNormalText(t *testing.T) {
	book := parseSynthetic(t, syntheticNovel(5, 40))
	if book.Quality.ChapterCount != 5 {
		t.Fatalf("章数 = %d，期望 5", book.Quality.ChapterCount)
	}
	checkCleanRatios(t, book.Quality)
	if math.Abs(book.Quality.StructureScore-1) > 1e-9 {
		t.Errorf("结构分 = %.4f，期望 1（章数合理、无空章、无离群章）", book.Quality.StructureScore)
	}
	checkCompleteness(t, book)
}

// checkCleanRatios 断言「干净正文」的三类比率全为 0。
func checkCleanRatios(t *testing.T, q model.QualityMetrics) {
	t.Helper()
	if q.GarbageChars != 0 || q.GarbageRatio != 0 {
		t.Errorf("干净正文的乱码 = %d 个 / %.4f，期望 0", q.GarbageChars, q.GarbageRatio)
	}
	if q.DroppedLines != 0 || q.NoiseRatio != 0 {
		t.Errorf("干净正文的噪声 = %d 行 / %.4f，期望 0", q.DroppedLines, q.NoiseRatio)
	}
	if q.RawLines == 0 {
		t.Error("RawLines 应统计到清洗器处理过的原始行")
	}
	if q.DuplicateRatio != 0 || q.DuplicateBytes != 0 {
		t.Errorf("各段互不相同，重复率 = %.4f，期望 0", q.DuplicateRatio)
	}
}

// checkCompleteness 断言完整性字段与章数、字数自洽。
func checkCompleteness(t *testing.T, book *model.ParsedBook) {
	t.Helper()
	q := book.Quality
	if q.TotalChars < book.WordCount {
		t.Errorf("正文字符数 %d 应不小于字数 %d", q.TotalChars, book.WordCount)
	}
	if q.AvgChapterChars != q.TotalChars/q.ChapterCount {
		t.Errorf("平均章长 = %d，期望 %d", q.AvgChapterChars, q.TotalChars/q.ChapterCount)
	}
}

// ---------- 含广告 / 群号 / 网址 / 分隔线 ----------

// TestQualityNoiseLines 覆盖噪声率：清洗器丢弃的行计入分子，清洗行为本身不变。
func TestQualityNoiseLines(t *testing.T) {
	book := parseSynthetic(t, noisyNovel(5, 10))
	q := book.Quality
	// 每章 4 行噪声 × 5 章 = 20 行；分母 = 20 行噪声 + 50 行正文。
	if q.DroppedLines != 20 {
		t.Errorf("丢弃行数 = %d，期望 20", q.DroppedLines)
	}
	if q.RawLines != 70 {
		t.Errorf("原始行数 = %d，期望 70（噪声行也计入分母）", q.RawLines)
	}
	if math.Abs(q.NoiseRatio-20.0/70.0) > 1e-9 {
		t.Errorf("噪声率 = %.4f，期望 %.4f", q.NoiseRatio, 20.0/70.0)
	}
	// 只增统计、不改清洗：广告行不能留在正文里。
	for _, c := range book.Chapters {
		for _, bad := range []string{"强鹿小说群", "example-shop", "98765432", "==============="} {
			if strings.Contains(c.Content, bad) {
				t.Errorf("第 %d 章正文仍含噪声 %q", c.Idx, bad)
			}
		}
	}
}

// TestQualityNoiseFilterDisabled 覆盖关闭 noise 过滤器：没有清洗器就没有统计。
func TestQualityNoiseFilterDisabled(t *testing.T) {
	opt := DefaultOptions()
	opt.DisableFilters = []string{"noise"}
	book, err := New(opt).ParseBytes("合成书名.txt", []byte(noisyNovel(3, 10)))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	q := book.Quality
	if q.RawLines != 0 || q.DroppedLines != 0 || q.NoiseRatio != 0 {
		t.Errorf("未启用清洗器时噪声统计应为 0，实际 %d / %d / %.4f", q.RawLines, q.DroppedLines, q.NoiseRatio)
	}
}

// ---------- 替换字符与私用区 ----------

// TestQualityGarbageRunes 覆盖乱码率口径：U+FFFD、私用区、非法 UTF-8 字节各算 1 个字符。
func TestQualityGarbageRunes(t *testing.T) {
	// 3 个乱码字符：U+FFFD、私用区 U+E123、非法字节 0xFF。
	content := "正常的一句话内容。\uFFFD私用区\uE123字符" + string([]byte{0xFF}) + "结束。"
	d := &Doc{Chapters: []model.ParsedChapter{{Content: content, WordCount: countWords(content)}}}
	q := computeQuality(d)
	if q.GarbageChars != 3 {
		t.Fatalf("乱码字符数 = %d，期望 3（U+FFFD / 私用区 / 非法字节各 1）", q.GarbageChars)
	}
	if q.TotalChars != runeLen(content) {
		t.Errorf("正文字符数 = %d，期望 %d（非法字节按 1 个字符计）", q.TotalChars, runeLen(content))
	}
	want := 3 / float64(q.TotalChars)
	if math.Abs(q.GarbageRatio-want) > 1e-9 {
		t.Errorf("乱码率 = %.6f，期望 %.6f", q.GarbageRatio, want)
	}
}

// TestQualityGarbageEndToEnd 覆盖端到端：正文里的 U+FFFD 会被计入乱码率。
func TestQualityGarbageEndToEnd(t *testing.T) {
	opt := DefaultOptions()
	opt.ForceEncoding = EncUTF8
	text := "第1章 合成标题\n这是带替换字符的正文\ufffd，仍然可以解析。\n第2章 合成标题\n这是正常正文段落，没有乱码。\n"
	book, err := New(opt).ParseBytes("合成书名.txt", []byte(text))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if book.Quality.GarbageChars != 1 {
		t.Errorf("乱码字符数 = %d，期望 1", book.Quality.GarbageChars)
	}
	if book.Quality.GarbageRatio <= 0 || book.Quality.GarbageRatio >= 1 {
		t.Errorf("乱码率 = %.4f，期望落在 (0,1)", book.Quality.GarbageRatio)
	}
}

// ---------- 段落高重复 ----------

// TestQualityDuplicateParagraphs 覆盖内部重复率：两章正文相同 → 第二次出现的段落全算重复。
func TestQualityDuplicateParagraphs(t *testing.T) {
	// 段内互不相同（含段号），整段正文在两章之间完全相同。
	body := ""
	for j := 0; j < 10; j++ {
		body += "这是第" + strconv.Itoa(j) + "段文字，在两章里完全重复出现，用来验证书内重复段落的统计口径。\n"
	}
	text := "第1章 合成标题\n" + body + "第2章 合成标题\n" + body
	book := parseSynthetic(t, text)
	q := book.Quality
	if q.DuplicateBytes == 0 {
		t.Fatal("重复段落的字节数为 0，期望 > 0")
	}
	// 重复的是两章正文，标题不重复：比率略低于 0.5。
	if q.DuplicateRatio < 0.45 || q.DuplicateRatio > 0.5 {
		t.Errorf("重复率 = %.4f，期望落在 [0.45, 0.5]", q.DuplicateRatio)
	}
	if q.ContentBytes <= q.DuplicateBytes {
		t.Errorf("正文字节 %d 应大于重复字节 %d", q.ContentBytes, q.DuplicateBytes)
	}
}

// ---------- 边界 ----------

// TestQualityEmptyDoc 覆盖空 Doc：全部记 0，不 panic、不产生 NaN。
func TestQualityEmptyDoc(t *testing.T) {
	q := computeQuality(&Doc{})
	if q != (model.QualityMetrics{}) {
		t.Errorf("空 Doc 的指标应全为 0，实际 %+v", q)
	}
}

// TestQualityConcurrentParse 覆盖并发解析：统计字段挂在每个文档自己的清洗器上，
// 包级共享的 fullCleaner 不累积计数（配合 -race 验证）。
func TestQualityConcurrentParse(t *testing.T) {
	text := []byte(noisyNovel(3, 10))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			book, err := New(DefaultOptions()).ParseBytes("合成书名.txt", text)
			if err != nil || book.Quality.DroppedLines == 0 || book.Quality.NoiseRatio <= 0 {
				t.Errorf("并发解析结果异常: err=%v quality=%+v", err, book.Quality)
			}
		}()
	}
	wg.Wait()
}
