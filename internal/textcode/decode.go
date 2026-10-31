package textcode

import "slices"

// Decode 把码本流 src 还原成 UTF-8，追加到 dst 并返回。
//
// 契约：
//   - 空输入返回 dst 本身；
//   - 流被截断、码位超出本表项数、转义段不完整或码点非法，一律返回 ErrCorrupt；
//     出错时 dst 里可能已写入解出的前缀，调用方应丢弃（正文场景下整块重读即可）；
//   - 解码不依赖任何外部状态，同一份字节永远解出同一份结果。
func Decode(dst, src []byte) ([]byte, error) {
	return decodeTable(builtin, dst, src)
}

// decodeTable 是 Decode 的本体。
//
// 每个字符最多展开成 4 字节 UTF-8，按 3 倍输入预留容量（1 字节码 → 3 字节汉字）；
// 4 字节 emoji 只从 3 字节码展开，比例更低，不会越界增长。
func decodeTable(t *Table, dst, src []byte) ([]byte, error) {
	if t == nil {
		return dst, tablef("解码时码表为空")
	}
	if len(src) == 0 {
		return dst, nil
	}
	dst = slices.Grow(dst, 3*len(src))
	for i := 0; i < len(src); {
		b := src[i]
		if b < singlesCap {
			r, ok := t.at(int(b))
			if !ok {
				return dst, corruptf("1 字节码 0x%02X 超出码表 %d 项", b, len(t.entries))
			}
			dst = appendRune(dst, r)
			i++
			continue
		}
		var err error
		if dst, i, err = t.decodeUnit(dst, src, i); err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// decodeUnit 解掉 src[i] 起的 2/3 字节码或转义段，返回新的 dst 与下一个位置。
func (t *Table) decodeUnit(dst, src []byte, i int) ([]byte, int, error) {
	switch b := src[i]; {
	case b < lead3Start:
		return t.decodePair(dst, src, i)
	case b < escapeByte:
		return decodeTriple(dst, src, i)
	default:
		return decodeEscape(dst, src, i)
	}
}

// decodePair 解一个 2 字节码。
func (t *Table) decodePair(dst, src []byte, i int) ([]byte, int, error) {
	if i+1 >= len(src) {
		return dst, i, corruptf("2 字节码在偏移 %d 处被截断", i)
	}
	code := singlesCap + int(src[i]-lead2Start)*256 + int(src[i+1])
	r, ok := t.at(code)
	if !ok {
		return dst, i, corruptf("2 字节码 %d 超出码表 %d 项", code, len(t.entries))
	}
	return appendRune(dst, r), i + 2, nil
}

// decodeTriple 解一个 3 字节码（直接就是码点）。
func decodeTriple(dst, src []byte, i int) ([]byte, int, error) {
	if i+2 >= len(src) {
		return dst, i, corruptf("3 字节码在偏移 %d 处被截断", i)
	}
	r := rune(src[i]&0x1F)<<16 | rune(src[i+1])<<8 | rune(src[i+2])
	if !validScalar(r) {
		return dst, i, corruptf("3 字节码在偏移 %d 处不是合法码点 U+%X", i, uint32(r))
	}
	return appendRune(dst, r), i + 3, nil
}

// decodeEscape 解一个转义段：跳过 0xFF 与长度，把原始字节原样搬到输出。
func decodeEscape(dst, src []byte, i int) ([]byte, int, error) {
	if i+1 >= len(src) {
		return dst, i, corruptf("转义段在偏移 %d 处缺少长度字节", i)
	}
	n := int(src[i+1])
	if n == 0 || i+2+n > len(src) {
		return dst, i, corruptf("转义段在偏移 %d 处段长 %d 非法（剩余 %d 字节）", i, n, len(src)-i-2)
	}
	return append(dst, src[i+2:i+2+n]...), i + 2 + n, nil
}

// appendRune 把 r 以 UTF-8 追加到 dst。
//
// 手写而不是调 utf8.AppendRune：解码是 1 MB 块的热路径，这里省一次函数调用与一层参数检查；
// 输入已由调用方校验过是合法码点，不需要再兜底。
func appendRune(dst []byte, r rune) []byte {
	switch u := uint32(r); {
	case u < 0x80:
		return append(dst, byte(u))
	case u < 0x800:
		return append(dst, 0xC0|byte(u>>6), 0x80|byte(u)&0x3F)
	case u < 0x10000:
		return append(dst, 0xE0|byte(u>>12), 0x80|byte(u>>6)&0x3F, 0x80|byte(u)&0x3F)
	default:
		return append(dst, 0xF0|byte(u>>18), 0x80|byte(u>>12)&0x3F,
			0x80|byte(u>>6)&0x3F, 0x80|byte(u)&0x3F)
	}
}
