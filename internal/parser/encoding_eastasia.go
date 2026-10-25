package parser

import (
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// decodeShiftJIS 解码 Shift-JIS。x/text 的解码器遇到非法字节时写 U+FFFD 而不报错，
// 所以「像不像日文」交给 japaneseLike 判断，这里只负责解码。
func decodeShiftJIS(b []byte) string {
	out, err := japanese.ShiftJIS.NewDecoder().Bytes(b)
	if err != nil {
		return ""
	}
	return string(out)
}

// japaneseLike 判断解码结果是否像日文：假名占比够高、半角片假名与替换字符够少。
//
// 排除误判的依据：中文 GB18030 文本被当 Shift-JIS 解码时几乎全是半角片假名
// （单字节 0xA1–0xDF 区间），假名占比为 0，替换字符明显偏多。
func japaneseLike(s string) bool {
	total, kana, halfWidth, bad := 0, 0, 0, 0
	for _, r := range s {
		total++
		switch {
		case r == utf8.RuneError:
			bad++
		case r >= 0x3041 && r <= 0x30FF: // 平假名 + 片假名
			kana++
		case r >= 0xFF61 && r <= 0xFF9F: // 半角片假名
			halfWidth++
		}
	}
	if total < 8 {
		return false
	}
	return kana*100 >= total*10 && halfWidth*100 <= total*20 && bad*100 <= total*5
}

// hasHKSCSExtension 判断字节流里是否有 HKSCS 扩展区的双字节序列。
//
// 标准 Big5 首字节从 0xA1 起，0x81–0xA0 首字节只出现在 HKSCS 扩展区；
// x/text 的 Big5 解码器按 WHATWG big5 码表实现，本身就覆盖 HKSCS 扩展字。
func hasHKSCSExtension(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		c0, c1 := b[i], b[i+1]
		if c0 < 0x81 || c0 > 0xA0 {
			continue
		}
		if (c1 >= 0x40 && c1 <= 0x7E) || (c1 >= 0xA1 && c1 <= 0xFE) {
			return true
		}
	}
	return false
}

// hkscsLike 判断 Big5 解码结果是否像带 HKSCS 扩展字的繁中文本：
// 必须命中扩展区、无替换字符、汉字占多数。
func hkscsLike(raw []byte, s string) bool {
	if !hasHKSCSExtension(raw) {
		return false
	}
	total, han, bad := 0, 0, 0
	for _, r := range s {
		total++
		switch {
		case r == utf8.RuneError:
			bad++
		case unicode.Is(unicode.Han, r):
			han++
		}
	}
	if total == 0 || bad > 0 {
		return false
	}
	return han*100 >= total*50
}

// big5Encoding 根据扩展区特征区分 Big5-HKSCS 与普通 Big5。
func big5Encoding(raw []byte, text string) string {
	if hkscsLike(raw, text) {
		return EncBig5HKSCS
	}
	return EncBig5
}
