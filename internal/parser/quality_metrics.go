package parser

import (
	"strings"
	"unicode/utf8"

	"norvels/internal/model"
)

// 质量指标的口径（全部 O(n)、单次遍历；40 MB 基准见 quality_metrics_test.go）：
//
//	乱码率（按字符）：分子是「无法解读的字符」——U+FFFD 替换字符（编码环节已经丢了内容）、
//	  私用区 U+E000–U+F8FF（字体映射残留）、非法 UTF-8 字节（每个字节记 1 个字符，与
//	  Go range 的 RuneError 对齐）；分母是全书正文字符数（rune 计，含乱码字符）。
//	噪声率（按行）：分子是清洗器丢弃的行（广告 / 群号 / 网址 / 分隔线 / 版权声明 /
//	  混淆水印），分母是清洗器处理过的非空原始行。空行不进分母，否则双倍行距的排版
//	  会把比率稀释一半。
//	结构分（0~1，越大越好）：countSanity(章数, 总字符) × (1 - 空章比例) × (1 - 离群章比例)。
//	内部重复率（按字节）：段落 = 章内以换行分隔的行；归一化 = 去掉全部空白字符；
//	  归一化后不足 dupMinRunes 个字符的段落不参与（短句重复是正常行文）；
//	  分子是第 2 次及以后出现的段落的原始字节数，分母是全书正文字节数。
//	完整性（原始计数）：章数 / 总字数 / 平均章长，供与旧版本逐项对照。
//
// 统计范围是「最终章节正文」——清洗、切章、后处理都做完之后读者真正读到的内容；
// 不含没成章的前置元信息段（书名 / 作者 / 简介），也不含已被丢弃的噪声行。
// 噪声的分母是清洗器处理过的行，离群章被再切时会重复处理一次（重复的行只进分母、
// 不产生新的丢弃），因此噪声率在结构异常的书里会略微偏低。

const (
	// dupMinRunes 是参与内部重复率统计的段落最小长度（归一化后按字符）。
	// 低于它的短句（如「他笑了。」）在正常行文里天然重复，计入只会稀释指标。
	dupMinRunes = 12
	// 私用区（BMP Private Use Area）：字体映射残留，正文里没有语义。
	puaStart = 0xE000
	puaEnd   = 0xF8FF
	// FNV-1a 64 位常量：给归一化段落算哈希用，不引哈希包，避免逐段分配。
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// computeQuality 计算一份解析产物的可解释质量指标。
func computeQuality(d *Doc) model.QualityMetrics {
	q := model.QualityMetrics{ChapterCount: len(d.Chapters)}
	// 按每章约 32 段预估重复段落表的容量：40 MB 的书有几十万段，让 map 反复扩容
	// 比多留几 MB 内存贵得多。
	scan := contentScan{seen: make(map[uint64]struct{}, len(d.Chapters)*32)}
	lens := make([]int, 0, len(d.Chapters))
	empty := 0
	for _, c := range d.Chapters {
		q.WordCount += c.WordCount
		if c.WordCount == 0 && isEmptyChapter(c) {
			empty++
		}
		lens = append(lens, scan.add(c.Content))
	}
	q.TotalChars, q.ContentBytes = scan.chars, scan.bytes
	q.GarbageChars, q.DuplicateBytes = scan.garbage, scan.dupBytes
	q.GarbageRatio = ratio(scan.garbage, scan.chars)
	q.DuplicateRatio = ratio(scan.dupBytes, scan.bytes)
	q.CountSanity = countSanity(q.ChapterCount, q.TotalChars)
	q.EmptyChapterRatio = ratio(empty, q.ChapterCount)
	q.OutlierChapterRatio = ratio(len(findOutliersByLen(lens)), q.ChapterCount)
	q.StructureScore = q.CountSanity * (1 - q.EmptyChapterRatio) * (1 - q.OutlierChapterRatio)
	if d.cl != nil && d.cl.stats != nil {
		q.RawLines, q.DroppedLines = d.cl.stats.raw, d.cl.stats.drop
	}
	q.NoiseRatio = ratio(q.DroppedLines, q.RawLines)
	if q.ChapterCount > 0 {
		q.AvgChapterChars = q.TotalChars / q.ChapterCount
	}
	return q
}

// contentScan 累积正文的字符 / 字节 / 乱码 / 重复段落统计，单次遍历完成。
type contentScan struct {
	chars    int
	bytes    int
	garbage  int
	dupBytes int
	seen     map[uint64]struct{} // 已出现段落的归一化哈希
}

// add 统计一段正文（一章）：按换行切段后逐段统计，返回该章的字符数。
func (s *contentScan) add(content string) int {
	before := s.chars
	s.bytes += len(content)
	for start := 0; start <= len(content); {
		nl := strings.IndexByte(content[start:], '\n')
		if nl < 0 {
			s.para(content[start:])
			break
		}
		s.para(content[start : start+nl])
		start += nl + 1
	}
	return s.chars - before
}

// para 统计一个段落的字符数、乱码字符数，并判定它与书内已有段落是否重复。
func (s *contentScan) para(p string) {
	chars, garbage, h, norm := scanPara(p)
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

// scanPara 一次遍历算出一个段落的字符数、乱码字符数与归一化哈希。
//
// 乱码字符不参与哈希：同一段文字里乱码位置不同，仍应被认成同一段。
func scanPara(p string) (chars, garbage int, h uint64, norm int) {
	h = fnvOffset64
	for i := 0; i < len(p); {
		r, size := utf8.DecodeRuneInString(p[i:])
		i += size
		chars++
		if isGarbageRune(r) {
			garbage++
			continue
		}
		if isSpaceRune(r) {
			continue
		}
		norm++
		h = (h ^ uint64(r)) * fnvPrime64
	}
	return chars, garbage, h, norm
}

// isSpaceRune 判断一个字符是否属于「排版空白」（归一化时剔除）：
// 空格 / 制表 / 回车 / 不间断空格 / 全角空格。用显式比较而不是 unicode.IsSpace——
// 正文以 CJK 为主，通用表查找在 40 MB 上要多花几十毫秒。
func isSpaceRune(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\u00a0', '\u3000':
		return true
	}
	return false
}

// isGarbageRune 判断一个已解码字符是否属于「乱码」：
//   - U+FFFD：合法编码的替换字符，或非法 UTF-8 字节——DecodeRuneInString 对每个
//     非法字节返回 (RuneError, 1)，所以非法字节按「每字节 1 个字符」计入；
//   - 私用区 U+E000–U+F8FF：字体映射残留，正文里没有语义。
func isGarbageRune(r rune) bool {
	return r == utf8.RuneError || (r >= puaStart && r <= puaEnd)
}

// ratio 返回 a/b；b<=0 时返回 0（没有内容时比率记 0，不产生 NaN）。
func ratio(a, b int) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) / float64(b)
}
