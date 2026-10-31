package textcode

import (
	"slices"
	"unicode/utf8"
)

// Encode 把 UTF-8 文本 src 编码成码本流，追加到 dst 并返回。
//
// 契约：
//   - 空输入返回 dst 本身（不分配、不写入）；
//   - 合法 UTF-8 字符按布局重编码，未进码表的字符用 3 字节兜底；
//   - 非法 UTF-8 字节原样封进转义段，Decode 后逐字节相同；
//   - 只有内置码表不可用时才返回错误，任意字节输入都不会报错、不会 panic。
func Encode(dst, src []byte) ([]byte, error) {
	return encodeTable(builtin, dst, src)
}

// encodeTable 是 Encode 的本体，单独拆出来便于测试 nil 码表这条兜底分支。
func encodeTable(t *Table, dst, src []byte) ([]byte, error) {
	if t == nil {
		return dst, tablef("编码时码表为空")
	}
	if len(src) == 0 {
		return dst, nil
	}
	dst = slices.Grow(dst, len(src))
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && size == 1 {
			// 非法字节：连同后面连续的非法字节一起封成一个转义段。
			dst, i = appendRawRun(dst, src, i)
			continue
		}
		dst = t.appendCode(dst, r)
		i += size
	}
	return dst, nil
}

// appendCode 按布局把一个字符写进 dst：命中 1 字节层 → 1 字节，命中 2 字节层 → 2 字节，
// 未进表 → 3 字节兜底。查不到码位不是错误，长尾字符永远有出路。
func (t *Table) appendCode(dst []byte, r rune) []byte {
	code, ok := t.code(r)
	if !ok {
		return appendUncoded(dst, r)
	}
	if code < singlesCap {
		return append(dst, byte(code))
	}
	q := code - singlesCap
	return append(dst, lead2Start+byte(q>>8), byte(q))
}

// appendUncoded 用 3 字节层写一个未进码表的合法码点：就是码点原样的 3 字节形态。
func appendUncoded(dst []byte, r rune) []byte {
	u := uint32(r)
	return append(dst, lead3Start+byte(u>>16), byte(u>>8), byte(u))
}

// appendRawRun 把 src[start:] 开头连续的非法 UTF-8 字节封进转义段，返回新的 dst 与下一个位置。
//
// 段长上限 255（长度字段只有一个字节）；超长的非法字节串会被切成多段。
func appendRawRun(dst, src []byte, start int) ([]byte, int) {
	end := start
	for end < len(src) && end-start < maxRawRun && isInvalidByte(src[end:]) {
		end++
	}
	dst = append(dst, escapeByte, byte(end-start))
	return append(dst, src[start:end]...), end
}

// isInvalidByte 报告 b 的首字节是否起不出一个合法的 UTF-8 序列。
// 合法编码的 U+FFFD 是 3 字节序列（size=3），只有 size=1 才是真脏字节。
func isInvalidByte(b []byte) bool {
	r, size := utf8.DecodeRune(b)
	return r == utf8.RuneError && size == 1
}
