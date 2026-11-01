package store

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"norvels/internal/model"
)

// 本文件是「从库里已存的正文重算质量分」的口径（设计 §5 的存量补齐）。
//
// 为什么需要它：0020 只加列，存量书的质量分是空的；判重拿不到旧版本的质量分就只能
// 走保守路径（判重复），版本择优对历史书等于没开。回填把几十 GB 正文解压一遍、
// 逐书算分写回（见 quality_backfill.go）。
//
// # 与解析期口径的关系
//
// 与 internal/parser 的 computeQuality 同式：乱码率按字符、内部重复率按段落字节、
// 结构分 = countSanity × (1-空章率) × (1-离群章率)。两处唯一的差别是**噪声维度**：
// 解析期噪声率来自清洗器丢掉了多少行，而库里存的是清洗之后的正文——噪声行在入库时
// 就已经丢了，回填只能记「残留噪声 = 0」。这条差别是刻意接受并写在文档里的：
// 噪声率恒为 0 会让「噪声率升 >=50%」对历史书永远成立（保守方向：新版本只要有噪声
// 就不推成升级候选），漏判一次升级候选比误推一次替换安全。
//
// 公式必须与 parser 一致，否则同一本书「导入时算的分」与「回填算的分」会不一样。
// 对照测试在 cmd/norvels（那里同时能引 parser 与 store）：把一本书的七列清空后回填，
// 逐项对比解析期质量分。
//
// 统计范围是**最终章节正文**（读者读到的内容），不含标题与元信息。

const (
	// dupMinRunes 是参与内部重复率统计的段落最小长度（归一化后按字符），与 parser 同值。
	dupMinRunes = 12
	// 私用区（BMP Private Use Area）：字体映射残留，正文里没有语义。
	puaStart = 0xE000
	puaEnd   = 0xF8FF
	// FNV-1a 64 位常量：给归一化段落算哈希（与 parser 同一组常量）。
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// computeStoredQuality 从章节正文重算质量分；正文全空返回 ok=false（这类书没有质量分可算）。
//
// 只是 builder 的便利包装：回填走流式（按块喂 addChapter，不把整本正文留在内存）。
func computeStoredQuality(chapters []model.ParsedChapter) (BookQuality, bool) {
	b := newStoredQualityBuilder(len(chapters))
	for i := range chapters {
		b.addChapter(chapters[i].Content)
	}
	return b.finish()
}

// storedQualityBuilder 逐章累积统计。回填按块流式喂章节：一本书的正文可能上百 MB，
// 但统计只需要「章长列表 + 段落哈希集」，正文本身用完即弃。
type storedQualityBuilder struct {
	scan     storedQualityScan
	lens     []int
	chapters int
	empty    int
}

// newStoredQualityBuilder 按预估章数预分配（回填能先拿到章节数，省掉扩容）。
func newStoredQualityBuilder(chapters int) *storedQualityBuilder {
	if chapters < 0 {
		chapters = 0
	}
	return &storedQualityBuilder{
		scan: storedQualityScan{seen: make(map[uint64]struct{}, chapters*32)},
		lens: make([]int, 0, chapters),
	}
}

// addChapter 累积一章正文。
func (b *storedQualityBuilder) addChapter(content string) {
	b.chapters++
	if isEmptyStoredChapter(content) {
		b.empty++
	}
	b.lens = append(b.lens, b.scan.add(content))
}

// finish 汇总成质量分；正文全空（一个字符都没有）返回 ok=false。
//
// 空章计入章数（它影响结构分），但不贡献字符与字节——与解析期同一口径。
func (b *storedQualityBuilder) finish() (BookQuality, bool) {
	if b.chapters == 0 || b.scan.chars == 0 {
		return BookQuality{}, false
	}
	q := BookQuality{
		Chapters:       b.chapters,
		Chars:          int64(b.scan.chars),
		GarbageRatio:   storedRatio(b.scan.garbage, b.scan.chars),
		DuplicateRatio: storedRatio(b.scan.dupBytes, b.scan.bytes),
		ComputedAt:     nowUTC(),
	}
	sanity := storedCountSanity(b.chapters, b.scan.chars)
	q.StructureScore = sanity * (1 - storedRatio(b.empty, b.chapters)) *
		(1 - storedRatio(len(storedOutliers(b.lens)), b.chapters))
	return q, true
}

// isEmptyStoredChapter 判断一章是否没有可读内容（与 parser 同口径：TrimSpace 之外还要按
// 字数判——装饰符章节 TrimSpace 去不掉，但字数确实是 0）。
func isEmptyStoredChapter(content string) bool {
	if strings.TrimSpace(content) == "" {
		return true
	}
	return countWords(content) == 0
}

// storedQualityScan 累积正文的字符 / 字节 / 乱码 / 重复段落统计（单次遍历）。
type storedQualityScan struct {
	chars    int
	bytes    int
	garbage  int
	dupBytes int
	seen     map[uint64]struct{}
}

// add 统计一章正文，返回该章的字符数。
func (s *storedQualityScan) add(content string) int {
	before := s.chars
	s.bytes += len(content)
	for start := 0; start <= len(content); {
		nl := strings.IndexByte(content[start:], 10)
		if nl < 0 {
			s.para(content[start:])
			break
		}
		s.para(content[start : start+nl])
		start += nl + 1
	}
	return s.chars - before
}

// para 统计一个段落：字符数、乱码字符数，并判定它与书内已有段落是否重复。
func (s *storedQualityScan) para(p string) {
	chars, garbage, h, norm := scanStoredPara(p)
	s.chars += chars
	s.garbage += garbage
	if norm < dupMinRunes {
		return
	}
	if _, ok := s.seen[h]; ok {
		s.dupBytes += len(p)
		return
	}
	s.seen[h] = struct{}{}
}

// scanStoredPara 一次遍历算出一个段落的字符数、乱码字符数与归一化哈希。
// 乱码字符不参与哈希：同一段文字里乱码位置不同，仍应被认成同一段。
func scanStoredPara(p string) (chars, garbage int, h uint64, norm int) {
	h = fnvOffset64
	for i := 0; i < len(p); {
		r, size := utf8.DecodeRuneInString(p[i:])
		i += size
		chars++
		if isStoredGarbageRune(r) {
			garbage++
			continue
		}
		if isStoredSpaceRune(r) {
			continue
		}
		norm++
		h = (h ^ uint64(r)) * fnvPrime64
	}
	return chars, garbage, h, norm
}

// isStoredSpaceRune 判断一个字符是否属于「排版空白」（归一化时剔除）：
// 空格 / 制表 / 回车 / 不间断空格 / 全角空格。
func isStoredSpaceRune(r rune) bool {
	switch r {
	case ' ', 9, '', 0x00a0, 0x3000:
		return true
	}
	return false
}

// isStoredGarbageRune 判断一个已解码字符是否属于「乱码」：U+FFFD（含非法 UTF-8 字节，
// DecodeRuneInString 对每个非法字节返回 (RuneError, 1)）与私用区。
func isStoredGarbageRune(r rune) bool {
	return r == utf8.RuneError || (r >= puaStart && r <= puaEnd)
}

// storedRatio 返回 a/b；b<=0 时返回 0（不产生 NaN）。
func storedRatio(a, b int) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// storedCountSanity 评估章数是否落在合理区间（与 parser 的 countSanity 同式）：
// 合理区间是 [总字数/8000, 总字数/1500]，落在区间内满分，偏少/偏多按比例衰减。
func storedCountSanity(chapters, totalChars int) float64 {
	if chapters <= 0 || totalChars <= 0 {
		return 0
	}
	lo := float64(totalChars) / 8000.0
	hi := float64(totalChars) / 1500.0
	n := float64(chapters)
	switch {
	case n < lo:
		return n / lo
	case n > hi:
		return hi / n
	}
	return 1.0
}

// 离群章的两个阈值（与 parser 同值）：
//
//	绝对阈值：超过 maxStoredOutlierChars 一定是切分失败留下的巨块；
//	统计阈值：超过 mean+2σ 且超过 max(2*median, minStoredOutlierChars) 才算离群。
const (
	maxStoredOutlierChars = 50000
	minStoredOutlierChars = 12000
)

// storedOutliers 返回内容明显偏长的章节下标（与 parser 的 findOutliersByLen 同式）。
func storedOutliers(lens []int) []int {
	if len(lens) == 0 {
		return nil
	}
	total := 0
	for _, n := range lens {
		total += n
	}
	mean := float64(total) / float64(len(lens))
	variance := 0.0
	for _, n := range lens {
		d := float64(n) - mean
		variance += d * d
	}
	sd := math.Sqrt(variance / float64(len(lens)))
	statLimit := mean + 2*sd
	hardLimit := math.Max(float64(2*storedMedian(lens)), minStoredOutlierChars)
	var out []int
	for i, n := range lens {
		fn := float64(n)
		if fn > maxStoredOutlierChars || (fn > statLimit && fn > hardLimit) {
			out = append(out, i)
		}
	}
	return out
}

// storedMedian 返回一组整数的中位数。
func storedMedian(v []int) int {
	if len(v) == 0 {
		return 0
	}
	cp := append([]int(nil), v...)
	sort.Ints(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}
