package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// 编码名常量。model.ParsedBook.Encoding 记录其中之一。
const (
	EncUTF8    = "utf-8"
	EncUTF8BOM = "utf-8-bom"
	EncGB18030 = "gb18030"
	EncBig5    = "big5"
	EncUTF16LE = "utf-16le"
	EncUTF16BE = "utf-16be"
	EncUnknown = "unknown"
)

// Decode 探测编码并解码，返回编码名与文本（已去 BOM、CRLF→LF）。
// 单独暴露出来便于针对编码探测做单元测试与排障。
func Decode(b []byte) (enc string, text string) {
	e, t, _ := decodeBytes(b)
	return e, t
}

// decodeBytes 是编码探测与解码的核心，返回编码名、文本与告警。
//
// 策略（按成本递增，绝大多数文件在前几步命中）：
//  1. BOM 判定：UTF-8 / UTF-16LE / UTF-16BE / UTF-32；
//  2. 大量 NUL 字节 → 无 BOM 的 UTF-16LE/BE；
//  3. 整文件 UTF-8 合法性（Go 的 utf8.Valid 很快）；
//  4. 整文件 GB18030 合法性（x/text 解码器）；
//  5. 整文件 Big5 合法性；
//  6. 以上都不成立 → 打分选最优候选，整文件按最优候选「尽力解码」（非法字节替换为 U+FFFD）。
//
// 关键点：必须对「整文件」做合法性校验，不能只看文件头。实测有 313 个文件前
// 300KB 无法按 GB18030 解码（截断处劈开了多字节字符），但整文件是合法 GB18030。
func decodeBytes(b []byte) (string, string, []string) {
	if len(b) == 0 {
		return EncUTF8, "", nil
	}

	// 1) BOM
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return EncUTF8BOM, decodeUTF8(b[3:]), nil
	case len(b) >= 4 && b[0] == 0xFF && b[1] == 0xFE && b[2] == 0x00 && b[3] == 0x00,
		len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0xFE && b[3] == 0xFF:
		return EncUTF16LE, decodeUTF16(b[4:], true).text, []string{"检测到 UTF-32 BOM，按 UTF-16LE 尽力解码"}
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		u := decodeUTF16(b[2:], true)
		var w []string
		if u.bad > 0 {
			w = append(w, "utf-16le: 存在孤立代理项或奇数尾字节")
		}
		return EncUTF16LE, u.text, w
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		u := decodeUTF16(b[2:], false)
		var w []string
		if u.bad > 0 {
			w = append(w, "utf-16be: 存在孤立代理项或奇数尾字节")
		}
		return EncUTF16BE, u.text, w
	}

	// 2) 无 BOM 的 UTF-16：纯 ASCII 的 UTF-16 文本可能是合法 UTF-8，
	//    所以要在 UTF-8 判定之前用 NUL 分布排除。
	if looksUTF16(b) {
		le := decodeUTF16(b, true)
		be := decodeUTF16(b, false)
		if le.bad*8 < len(b)/2 || be.bad*8 < len(b)/2 {
			if textQuality(le.text) >= textQuality(be.text) {
				return EncUTF16LE, le.text, []string{"无 BOM，按 UTF-16LE 解码"}
			}
			return EncUTF16BE, be.text, []string{"无 BOM，按 UTF-16BE 解码"}
		}
	}

	// 3) UTF-8
	if utf8.Valid(b) {
		return EncUTF8, string(b), nil
	}

	// 4) GB18030
	gbText, gbOK := decodeGB18030(b)
	if gbOK {
		// GB18030 与 Big5 的字节空间有重叠：若 GB18030 解出的文本质量偏低，
		// 再看 Big5 能否解得更像正常中文。
		if q := textQuality(gbText); q < 150 {
			if bt, bigOK := decodeBig5(b); bigOK && textQuality(bt) > q+20 {
				return EncBig5, bt, nil
			}
		}
		return EncGB18030, gbText, nil
	}

	// 5) Big5
	if bt, ok := decodeBig5(b); ok {
		return EncBig5, bt, nil
	}

	// 6) 都不合法：按文本质量挑最优，尽力解码并告警。
	bestEnc, bestText := EncUnknown, decodeUTF8(b)
	bestScore := textQuality(bestText)
	if s := textQuality(gbText); s > bestScore {
		bestEnc, bestText, bestScore = EncGB18030, gbText, s
	}
	if bt, _ := decodeBig5(b); bt != "" {
		if s := textQuality(bt); s > bestScore {
			bestEnc, bestText, bestScore = EncBig5, bt, s
		}
	}
	return bestEnc, bestText, []string{"编码无法严格识别，已按最优候选尽力解码（可能含乱码）"}
}

// looksUTF16 用 NUL 字节的位置分布判断是否可能是无 BOM 的 UTF-16。
func looksUTF16(b []byte) bool {
	n := len(b)
	if n < 8 {
		return false
	}
	sample := n
	if sample > 8192 {
		sample = 8192
	}
	evenNUL, oddNUL, nul := 0, 0, 0
	for i := 0; i < sample; i++ {
		if b[i] != 0 {
			continue
		}
		nul++
		if i%2 == 0 {
			evenNUL++
		} else {
			oddNUL++
		}
	}
	if nul*8 < sample {
		return false
	}
	dom := evenNUL
	if oddNUL > dom {
		dom = oddNUL
	}
	return dom*10 >= nul*9
}

type utf16Result struct {
	text string
	bad  int
}

// decodeUTF16 解码 UTF-16（little 表示小端），非法序列记为 U+FFFD 并计数。
func decodeUTF16(b []byte, little bool) utf16Result {
	var sb strings.Builder
	sb.Grow(len(b))
	bad := 0
	for i := 0; i+1 < len(b); i += 2 {
		var u uint16
		if little {
			u = uint16(b[i]) | uint16(b[i+1])<<8
		} else {
			u = uint16(b[i])<<8 | uint16(b[i+1])
		}
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			if i+3 < len(b) {
				var u2 uint16
				if little {
					u2 = uint16(b[i+2]) | uint16(b[i+3])<<8
				} else {
					u2 = uint16(b[i+2])<<8 | uint16(b[i+3])
				}
				if u2 >= 0xDC00 && u2 <= 0xDFFF {
					r := 0x10000 + (rune(u)-0xD800)<<10 + (rune(u2) - 0xDC00)
					sb.WriteRune(r)
					i += 2
					continue
				}
			}
			sb.WriteRune(utf8.RuneError)
			bad++
		case u >= 0xDC00 && u <= 0xDFFF:
			sb.WriteRune(utf8.RuneError)
			bad++
		default:
			sb.WriteRune(rune(u))
		}
	}
	if len(b)%2 == 1 {
		sb.WriteRune(utf8.RuneError)
		bad++
	}
	return utf16Result{text: sb.String(), bad: bad}
}

// decodeUTF8 解码 UTF-8，非法字节替换为 U+FFFD。
func decodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			sb.WriteRune(utf8.RuneError)
			i++
			continue
		}
		sb.Write(b[i : i+size])
		i += size
	}
	return sb.String()
}

// decodeGB18030 解码 GB18030（GBK 的超集）。第二个返回值表示输入是否完全合法。
func decodeGB18030(b []byte) (string, bool) {
	dec := simplifiedchinese.GB18030.NewDecoder()
	out, err := dec.Bytes(b)
	if err == nil {
		return string(out), true
	}
	s, _ := decodeLossy(simplifiedchinese.GB18030.NewDecoder(), b)
	return s, false
}

// decodeBig5 解码 Big5。第二个返回值表示输入是否完全合法。
func decodeBig5(b []byte) (string, bool) {
	dec := traditionalchinese.Big5.NewDecoder()
	out, err := dec.Bytes(b)
	if err == nil {
		return string(out), true
	}
	s, _ := decodeLossy(traditionalchinese.Big5.NewDecoder(), b)
	return s, false
}

// decodeLossy 尽力解码：非法字节替换为 U+FFFD 后继续。
func decodeLossy(dec *encoding.Decoder, b []byte) (string, bool) {
	t := dec.Transformer
	out := make([]byte, 0, len(b)+len(b)/2)
	buf := make([]byte, 64*1024)
	src := b
	valid := true
	for len(src) > 0 {
		nDst, nSrc, err := t.Transform(buf, src, true)
		out = append(out, buf[:nDst]...)
		if nSrc > 0 {
			src = src[nSrc:]
		}
		if err == nil {
			break
		}
		if err == transform.ErrShortDst {
			continue
		}
		// 非法字节序列：替换一个 U+FFFD 并前进 1 字节
		out = appendRune(out, utf8.RuneError)
		valid = false
		if nSrc == 0 {
			src = src[1:]
		}
	}
	return string(out), valid
}

// appendRune 把码位以 UTF-8 追加到缓冲区。
func appendRune(dst []byte, r rune) []byte {
	switch {
	case r < 0x80:
		return append(dst, byte(r))
	case r < 0x800:
		return append(dst, 0xC0|byte(r>>6), 0x80|byte(r)&0x3F)
	case r >= 0x10000:
		return append(dst, 0xF0|byte(r>>18), 0x80|byte(r>>12)&0x3F, 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
	default:
		return append(dst, 0xE0|byte(r>>12), 0x80|byte(r>>6)&0x3F, 0x80|byte(r)&0x3F)
	}
}

// 常用字集合：用于区分 GB18030 与 Big5 的「错误但合法」解码。
const commonHanzi = "的一是不了在人有我他这个们中来上大为和国地到以说时要就出会可也你对生能而子那得于着下自之年过发后作里用道行所然家种事成方多经么去法学如都同现当没动面起看定天分还进好小部其些主样理心她本前开但因只从想实日军者意无力它与长把机十民第公此已工使情明性知全三又关点正业外将两高间由问很最重并物手应战向头文体政美相见被利什二等产或新己制身果加西斯月话合回特代内信表化老给世位次度门任常先海通教儿原东声提立及比员解水名真论处走义各入几口认条平系气题活尔更别打女变四神总何电数安少报才结反受目太量再感建务做接必场件计管期市直德资命山金指克许统区保至队形社便空决治展马科司五基眼书非则听白却界达光放强即像难且权思王象完设式色路记南品住告类求据程北边死张该交规万取拉格望觉术领共确传师观清今切院让识候带导争运笑飞风步改收根干造言联持组每济车亲极林服快办议往元英士证近失转夫令准布始怎呢存未远叫台单影具罗字爱击流备兵连调深商算质团集百需价花党华城石级整府离况亚请技际约示复病息究线似官火断精满支视消越器容照须九增研写称企八功吗包片史委乎查轻易早曾除农找装广显吧阿李标谈吃图念六引历首医局突专费号尽另周较注语仅考落青随选列武红响虽推势参希古众构房半节土投某案黑维革划敌致陈律足态护七兴派孩验责营星够章音跟志底站严巴例防族供效续施留讲型料终答紧黄绝奇察母京段依批群项故按河米围江织害斗双境客纪采举杀攻父苏显"

// commonSet 把常用字常量展开成查表数组，避免逐字串扫描。
var commonSet = func() *[0x10000]bool {
	var t [0x10000]bool
	for _, r := range commonHanzi {
		if r < 0x10000 {
			t[r] = true
		}
	}
	return &t
}()

// textQuality 粗略评估一段文本「像不像正常中文」：
// 常用汉字、CJK 标点加分，替换字符与控制字符重罚。返回值越大越好。
func textQuality(s string) float64 {
	if s == "" {
		return 0
	}
	total, han, cjkPunct, common, bad := 0, 0, 0, 0, 0
	limit := 20000
	for _, r := range s {
		if total >= limit {
			break
		}
		total++
		switch {
		case r == utf8.RuneError:
			bad++
		case r < 0x20 && r != '\n' && r != '\r' && r != '\t':
			bad++
		case unicode.Is(unicode.Han, r):
			han++
			if r < 0x10000 && commonSet[r] {
				common++
			}
		case r >= 0x3000 && r <= 0x303F:
			cjkPunct++
		case r >= 0xFF00 && r <= 0xFF65:
			cjkPunct++
		}
	}
	f := float64(total)
	return (float64(han)*100+float64(cjkPunct)*20+float64(common)*120)/f - float64(bad)*500/f
}
