package parser

import (
	"strings"
	"unicode/utf8"
)

// 编码分数刻度（同一刻度上比较，互不重叠的区间保证优先级语义）：
//
//	BOM 判定        1000
//	无 BOM UTF-16    850 + 质量
//	UTF-8 合法       800
//	Shift-JIS        760（假名特征命中时；固定分，压过 GB/Big5 上限 750）
//	Big5-HKSCS       710 + 质量（仅比 Big5 高 10，带 HKSCS 扩展区时优先记账）
//	GB18030/Big5 合法 700 + 质量（上限 740）
//	尽力解码          100 + 质量（上限 140）
//
// 这样「BOM > UTF-16 > UTF-8 > Shift-JIS > Big5-HKSCS > GB/Big5 > 兜底」的判定顺序与原实现一致。
const (
	scoreBOM      = 1000.0
	scoreUTF16    = 850.0
	scoreUTF8     = 800.0
	scoreShiftJIS = 760.0
	scoreHKSCS    = 710.0
	scoreLegacy   = 700.0
	scoreLossy    = 100.0
	scoreMaxQual  = 400.0
)

// qualityScore 把文本质量分压进 [0, 40) 的区间，避免跨档位翻转。
func qualityScore(base, q float64) float64 {
	if q > scoreMaxQual {
		q = scoreMaxQual
	}
	if q < 0 {
		q = 0
	}
	return base + q/10
}

// utf8Codec 处理无 BOM 的 UTF-8（含非法字节时尽力解码）。
type utf8Codec struct{}

func (utf8Codec) Name() string { return EncUTF8 }

func (utf8Codec) Try(raw []byte) (string, float64) {
	if utf8.Valid(raw) {
		return string(raw), scoreUTF8
	}
	text := decodeUTF8(raw)
	return text, qualityScore(scoreLossy, textQuality(text))
}

// utf8BOMCodec 处理带 UTF-8 BOM 的文件（BOM 是确定性特征，直接给最高分）。
type utf8BOMCodec struct{}

func (utf8BOMCodec) Name() string { return EncUTF8BOM }

func (utf8BOMCodec) Try(raw []byte) (string, float64) {
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		return decodeUTF8(raw[3:]), scoreBOM
	}
	return "", 0
}

// gb18030Codec 处理 GB18030 / GBK。
//
// 与 Big5 的字节空间有重叠，所以严格合法时要和 Big5 交叉检验：GB 解出的文本
// 质量偏低而 Big5 明显更像正常中文时主动降分，让 Big5 胜出。
type gb18030Codec struct{}

func (gb18030Codec) Name() string { return EncGB18030 }

func (gb18030Codec) Try(raw []byte) (string, float64) {
	text, ok := decodeGB18030(raw)
	if !ok {
		return text, qualityScore(scoreLossy, textQuality(text))
	}
	q := textQuality(text)
	if q < 150 {
		if bt, bigOK := decodeBig5(raw); bigOK && textQuality(bt) > q+20 {
			return text, qualityScore(scoreLossy, q)
		}
	}
	return text, qualityScore(scoreLegacy, q)
}

// big5Codec 处理繁体 Big5。
type big5Codec struct{}

func (big5Codec) Name() string { return EncBig5 }

func (big5Codec) Try(raw []byte) (string, float64) {
	text, ok := decodeBig5(raw)
	if !ok {
		return text, qualityScore(scoreLossy, textQuality(text))
	}
	return text, qualityScore(scoreLegacy, textQuality(text))
}

// shiftJISCodec 处理日文 Shift-JIS。
//
// 命中条件不是「解码不报错」（x/text 对非法字节不报错），而是解码结果的假名
// 特征；中文 GB18030 被当 Shift-JIS 解码时几乎全是半角片假名，会被挡掉。
type shiftJISCodec struct{}

func (shiftJISCodec) Name() string { return EncShiftJIS }

func (shiftJISCodec) Try(raw []byte) (string, float64) {
	text := decodeShiftJIS(raw)
	if !japaneseLike(text) {
		return text, 0
	}
	return text, scoreShiftJIS
}

// big5HKSCSCodec 处理繁体 Big5-HKSCS（港台常用，含 Big5 之外的扩展字）。
//
// x/text 的 Big5 解码器按 WHATWG big5 码表实现，已覆盖 HKSCS 扩展字；这里只负责
// 按扩展区特征把它和普通 Big5 区分开，避免普通 Big5 被改名。
type big5HKSCSCodec struct{}

func (big5HKSCSCodec) Name() string { return EncBig5HKSCS }

func (big5HKSCSCodec) Try(raw []byte) (string, float64) {
	text, ok := decodeBig5(raw)
	if !ok || !hkscsLike(raw, text) {
		return text, 0
	}
	return text, qualityScore(scoreHKSCS, textQuality(text))
}

// utf16LECodec 处理 UTF-16LE（带 BOM 与无 BOM 两种）。
type utf16LECodec struct{}

func (utf16LECodec) Name() string { return EncUTF16LE }

func (utf16LECodec) Try(raw []byte) (string, float64) {
	// UTF-32LE BOM 按 UTF-16LE 尽力解码，与旧实现一致。
	if len(raw) >= 4 && raw[0] == 0xFF && raw[1] == 0xFE && raw[2] == 0x00 && raw[3] == 0x00 {
		return decodeUTF16(raw[4:], true).text, scoreBOM
	}
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		r := decodeUTF16(raw[2:], true)
		return r.text, scoreBOM
	}
	if !looksUTF16(raw) {
		return "", 0
	}
	r := decodeUTF16(raw, true)
	if r.bad*8 >= len(raw)/2 {
		return "", 0
	}
	return r.text, qualityScore(scoreUTF16, textQuality(r.text))
}

// utf16BECodec 处理 UTF-16BE（带 BOM 与无 BOM 两种）。
type utf16BECodec struct{}

func (utf16BECodec) Name() string { return EncUTF16BE }

func (utf16BECodec) Try(raw []byte) (string, float64) {
	if len(raw) >= 4 && raw[0] == 0x00 && raw[1] == 0x00 && raw[2] == 0xFE && raw[3] == 0xFF {
		return decodeUTF16(raw[4:], false).text, scoreBOM
	}
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		return decodeUTF16(raw[2:], false).text, scoreBOM
	}
	if !looksUTF16(raw) {
		return "", 0
	}
	r := decodeUTF16(raw, false)
	if r.bad*8 >= len(raw)/2 {
		return "", 0
	}
	return r.text, qualityScore(scoreUTF16, textQuality(r.text))
}

// registerBuiltinCodecs 注册全部内置编码策略。
func registerBuiltinCodecs() {
	RegisterCodec(utf8BOMCodec{})
	RegisterCodec(utf16LECodec{})
	RegisterCodec(utf16BECodec{})
	RegisterCodec(utf8Codec{})
	RegisterCodec(gb18030Codec{})
	RegisterCodec(big5Codec{})
	RegisterCodec(big5HKSCSCodec{})
	RegisterCodec(shiftJISCodec{})
}

// decodeByCodecs 用注册表里的编码策略选最优解码结果。
//
// 与旧 decodeBytes 的差异：这里不再有硬编码的分支顺序，顺序完全由分数决定；
// 分数区间保证了等价语义。候选数记录进 stats 便于排障。
func decodeByCodecs(raw []byte, forced string, stats func(string, any)) (text, enc string, score float64) {
	if len(raw) == 0 {
		return "", EncUTF8, 0
	}
	list := Codecs()
	if forced != "" {
		for _, c := range list {
			if c.Name() == forced {
				t, s := c.Try(raw)
				return t, forced, s
			}
		}
		stats("codec_unknown_forced", forced)
	}
	bestText, bestEnc, bestScore := "", EncUTF8, -1.0
	cand := 0
	for _, c := range list {
		t, s := c.Try(raw)
		if s <= 0 {
			continue
		}
		cand++
		if s > bestScore {
			bestText, bestEnc, bestScore = t, c.Name(), s
		}
	}
	if bestScore <= 0 {
		// 所有策略都给了 0 分（例如强制编码不存在）：按 UTF-8 尽力解码，保证能入库。
		return decodeUTF8(raw), EncUnknown, 0
	}
	stats("codec_candidates", cand)
	return bestText, bestEnc, bestScore
}

// trimCodecText 去掉解码后残留的 BOM 字符。
func trimCodecText(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}
