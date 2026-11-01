package store

import (
	"hash/fnv"
	"sync"
	"unicode"
	"unicode/utf8"
)

// 本文件是「章级内容指纹」的纯函数口径（设计 §4）：正文归一化 → 64 位 FNV-1a。
//
// 为什么用 FNV-1a 而不是 SHA：指纹只用于库内查重，不防篡改、不需要抗碰撞证明；
// hash/fnv 是标准库、单遍、无额外依赖。64 位空间下百万级章节的生日碰撞概率极低，
// 真撞了也只会让一本书落进「疑似相关」或「同一本书」，不会误删任何数据。
//
// 指纹**不含书名/作者/路径/编码**：同样的正文换个文件名必须能互相命中，这正是去重的目的。
//
// 归一化与哈希一趟做完（appendNormalized 写进池化缓冲、最后整体喂给 FNV），
// 因为判定一本 40 MB 的书要在 200 ms 内跑完：逐 rune 调 EncodeRune + 逐 rune Write
// 会把同样的正文跑成两百多毫秒。纯函数口径由 NormalizeChapterText 暴露，两者共用
// 同一份归一化实现（不存在第二套规则）。

// 归一化规则（NormalizeChapterText 与 chapterFingerprint 共用）：
//   - 空白（空格/换行/制表/全角空格等）全部丢弃；
//   - 中文标点折成半角同名符号（表见 cjkPunctCanonical）；
//   - 全角 ASCII（U+FF01–U+FF5E）折成半角；
//   - 其它字符原样保留。

// NormalizeChapterText 归一化一章正文：删掉全部空白字符，把全角/半角标点统一到同一形态。
//
// 归一化只抹掉「排版差异」。**不做**繁简转换、异体字归并、错别字纠正——那需要词表与
// 语义判断，属于质量分（设计 §5）而不是去重口径；混进来会让「命中」不再可解释。
func NormalizeChapterText(s string) string {
	return string(appendNormalized(make([]byte, 0, len(s)), s))
}

// ChapterHash 对**已归一化**的正文取 64 位 FNV-1a，返回 int64。
//
// 为什么是 int64：SQLite 的 INTEGER 是有符号 64 位，指纹列按 hash INTEGER 存；
// Go 侧一律用 int64 表达（无符号值按补码落库，转换可逆且两侧口径一致）。
func ChapterHash(normalized string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalized))
	return int64(h.Sum64())
}

// maxPooledScratch 是允许回到池里的缓冲上限：超长章节（几 MB）用完就丢，
// 不把大块内存长期挂在 sync.Pool 上。
const maxPooledScratch = 1 << 20

// fpScratch 是归一化缓冲池：判定一本两千章的书会反复申请同一量级的中间缓冲，复用省下分配与 GC。
var fpScratch = sync.Pool{New: func() any {
	buf := make([]byte, 0, 32<<10)
	return &buf
}}

// chapterFingerprint 返回一章的指纹与字节权重，空白正文返回 ok=false。
//
// bytes 取**归一化前**的原始字节数：报告里的「命中字节」要能对上这本书自己的体积，
// 用归一化后的长度会让「同一本书换个换行风格」的体积对不上。
func chapterFingerprint(content string) (hash, bytes int64, ok bool) {
	p := fpScratch.Get().(*[]byte)
	buf := appendNormalized((*p)[:0], content)
	if len(buf) == 0 {
		*p = buf
		fpScratch.Put(p)
		return 0, 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(buf)
	if cap(buf) <= maxPooledScratch {
		*p = buf
		fpScratch.Put(p)
	}
	return int64(h.Sum64()), int64(len(content)), true
}

// appendNormalized 把归一化后的正文字节追加到 dst 并返回。
//
// 三段式，目标是「中文正文不付出逐 rune 的代价」：
//  1. ASCII 字节直接判空白后原样过；
//  2. 首字节落在 0xE4–0xE9 的三字节序列（U+4000–U+9FFF，中文正文主体）**不解码**，
//     直接跨过 3 字节——这个区间里既没有空白也没有要折的标点，解码是白花钱；
//  3. 其余字符解码后逐条判定，需要改写时才把前面攒下的原样段刷出去。
//
// 原样段只在遇到「要删/要折」的字符时刷一次，所以一大段正文通常是一次 memmove。
func appendNormalized(dst []byte, s string) []byte {
	start := 0 // 当前「原样拷贝」区间的起点
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c < utf8.RuneSelf:
			if !asciiSpace(c) {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			i++
			start = i
		case c >= cjkByteLow && c <= cjkByteHigh:
			i += utf8.UTFMax - 1 // U+4000–U+9FFF：三字节、无需解码
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if isPlainRune(r) {
				i += size
				continue
			}
			dst = append(dst, s[start:i]...)
			dst = appendCanonical(dst, r, s[i:i+size])
			i += size
			start = i
		}
	}
	return append(dst, s[start:]...)
}

// isPlainRune 报告一个非 ASCII 字符是否既不需要删（空白）也不需要折（标点表内）。
func isPlainRune(r rune) bool {
	switch {
	case r >= cjkStart && r <= cjkEnd, r >= cjkExtAStart && r <= cjkExtAEnd:
		return true // 中文正文主体：这里不可能出现空白或要折的标点
	case r < 0x2000 || r > 0xFF5E:
		return !unicode.IsSpace(r)
	}
	return false
}

// appendCanonical 处理需要判定的字符：空白丢弃、标点折半角、其余原样追加。
func appendCanonical(dst []byte, r rune, raw string) []byte {
	if unicode.IsSpace(r) {
		return dst
	}
	if c, ok := cjkPunctCanonical[r]; ok {
		return append(dst, byte(c))
	}
	if r >= 0xFF01 && r <= 0xFF5E {
		return append(dst, byte(r-0xFEE0))
	}
	return append(dst, raw...)
}

// asciiSpace 是 ASCII 空白判定（空格/换行/制表/回车/垂直制表/换页）。
func asciiSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '\v' || c == '\f'
}

// CJK 区段：中文正文的主体，归一化时既非空白也不在标点表里（快路径的唯一依据）。
const (
	cjkStart     = 0x4E00
	cjkEnd       = 0x9FFF
	cjkExtAStart = 0x3400
	cjkExtAEnd   = 0x4DBF
	// 首字节 0xE4–0xE9 恰好覆盖 U+4000–U+9FFF（含 CJK 扩展 A 与基本区），
	// 且不含 Unicode 空白与标点表里的任何字符——按字节跳过的依据。
	cjkByteLow  = 0xE4
	cjkByteHigh = 0xE9
)

// cjkPunctCanonical 把中文（全角）标点折成半角同名符号。
//
// 表里只放「跨书排版差异最常见」的那批：句号逗号顿号分号冒号问号感叹号、括号、
// 书名/引号、破折号与省略号。没登记的字符原样保留——宁可少折几种，也不把
// 内容字符卷进归一化（那会把不同的字折成同一个，制造假的重复）。
var cjkPunctCanonical = map[rune]rune{
	'。': '.', '，': ',', '、': ',', '；': ';', '：': ':', '？': '?', '！': '!',
	'（': '(', '）': ')', '【': '[', '】': ']', '「': '"', '」': '"', '『': '"', '』': '"',
	'《': '<', '》': '>', '〈': '<', '〉': '>', '—': '-', '…': '.', '·': '.', '～': '~',
}
