package parser

import (
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// 样本全部是虚构文本，不含任何真实语料：日文一句原创短句，繁中一段原创港式对白。
const (
	fakeJapanese = "ゆきの降る町で、ねこがまどべにいる。"
	fakeHKSCS    = "佢哋喺公園睇花，嗰個細路笑得好開心。"
)

// encodeSample 用给定编码器把虚构文本编成字节，模拟磁盘上的文件。
func encodeSample(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("编码样本失败: %v", err)
	}
	return b
}

// TestCodecShiftJIS 覆盖 Shift-JIS 的命中与「中文不当日文」两条路径。
func TestCodecShiftJIS(t *testing.T) {
	raw := encodeSample(t, japanese.ShiftJIS, fakeJapanese)
	if utf8.Valid(raw) {
		t.Fatal("样本不应是合法 UTF-8，否则测不出探测顺序")
	}
	c := shiftJISCodec{}
	if c.Name() != EncShiftJIS {
		t.Errorf("Name = %q", c.Name())
	}
	text, score := c.Try(raw)
	if text != fakeJapanese || score != scoreShiftJIS {
		t.Errorf("Shift-JIS 解码 = %q（分 %v）", text, score)
	}
	// 中文 GB18030 字节被当 Shift-JIS 解码时是半角片假名，必须放弃。
	zh := encodeSample(t, simplifiedchinese.GB18030, "春天来了，村子里的孩子们在河边放风筝。")
	if text, score := c.Try(zh); score != 0 || text == "" {
		t.Errorf("中文不应判为 Shift-JIS: %q %v", text, score)
	}
}

// TestDecodeShiftJISOrder 断言 Shift-JIS 样本不会被 UTF-8 / GB18030 抢走。
func TestDecodeShiftJISOrder(t *testing.T) {
	raw := encodeSample(t, japanese.ShiftJIS, fakeJapanese)
	text, enc, _ := decodeByCodecs(raw, "", func(string, any) {})
	if enc != EncShiftJIS || text != fakeJapanese {
		t.Errorf("codec 探测 = %q %q", enc, text)
	}
	enc, text = Decode(raw)
	if enc != EncShiftJIS || text != fakeJapanese {
		t.Errorf("Decode = %q %q", enc, text)
	}
	// 强制编码仍可用（分数为 0 时不参与竞争，但强制路径要拿到文本）。
	if text, enc, _ := decodeByCodecs(raw, EncShiftJIS, func(string, any) {}); enc != EncShiftJIS || text != fakeJapanese {
		t.Errorf("强制 Shift-JIS = %q %q", enc, text)
	}
}

// TestCodecBig5HKSCS 覆盖 HKSCS 扩展区命中、普通 Big5 不被改名两条路径。
func TestCodecBig5HKSCS(t *testing.T) {
	raw := encodeSample(t, traditionalchinese.Big5, fakeHKSCS)
	if !hasHKSCSExtension(raw) || utf8.Valid(raw) {
		t.Fatalf("样本应落在 HKSCS 扩展区且非 UTF-8: % X", raw)
	}
	c := big5HKSCSCodec{}
	if c.Name() != EncBig5HKSCS {
		t.Errorf("Name = %q", c.Name())
	}
	text, score := c.Try(raw)
	if text != fakeHKSCS || score <= 0 {
		t.Errorf("HKSCS 解码 = %q（分 %v）", text, score)
	}
	// 普通 Big5（首字节 0xA1 起）不进 HKSCS 分支。
	plain := []byte{0xA4, 0xA4, 0xA4, 0xE5} // "中文"
	if _, score := c.Try(plain); score != 0 {
		t.Errorf("普通 Big5 不应判为 HKSCS: %v", score)
	}
	if enc := big5Encoding(plain, "中文"); enc != EncBig5 {
		t.Errorf("普通 Big5 归类 = %q", enc)
	}
}

// TestDecodeBig5HKSCSOrder 断言 HKSCS 样本不会被 GB18030 / UTF-8 / Big5 抢走。
func TestDecodeBig5HKSCSOrder(t *testing.T) {
	raw := encodeSample(t, traditionalchinese.Big5, fakeHKSCS)
	text, enc, _ := decodeByCodecs(raw, "", func(string, any) {})
	if enc != EncBig5HKSCS || text != fakeHKSCS {
		t.Errorf("codec 探测 = %q %q", enc, text)
	}
	enc, text = Decode(raw)
	if enc != EncBig5HKSCS || text != fakeHKSCS {
		t.Errorf("Decode = %q %q", enc, text)
	}
	// 现有 Big5 样本保持原判，不被新档位抢走。
	plain := []byte{0xA4, 0xA4, 0xA4, 0xE5}
	if _, enc, _ := decodeByCodecs(plain, "", func(string, any) {}); enc != EncBig5 {
		t.Errorf("普通 Big5 探测 = %q", enc)
	}
	// 现有 GB18030 样本仍判 GB18030。
	zh := encodeSample(t, simplifiedchinese.GB18030, "春天来了，村子里的孩子们在河边放风筝。")
	if _, enc, _ := decodeByCodecs(zh, "", func(string, any) {}); enc != EncGB18030 {
		t.Errorf("GB18030 探测 = %q", enc)
	}
	if _, score := (big5HKSCSCodec{}).Try(zh); score != 0 {
		t.Errorf("GB18030 不应判为 HKSCS: %v", score)
	}
}

// TestEastAsiaHelpers 覆盖两个特征函数的边界分支。
func TestEastAsiaHelpers(t *testing.T) {
	if japaneseLike("短") {
		t.Error("过短文本不应判为日文")
	}
	if japaneseLike("ｱｲｳｴｵｶｷｸｹｺ") {
		t.Error("半角片假名不应判为日文")
	}
	if !japaneseLike("ひらがなとカタカナの文章です") {
		t.Error("假名占比高的文本应判为日文")
	}
	if !hasHKSCSExtension([]byte{0x87, 0x40}) || hasHKSCSExtension([]byte{0xA4, 0xA4}) {
		t.Error("扩展区判定错误")
	}
	if !hkscsLike([]byte{0x9D, 0xF5}, "中文") {
		t.Error("扩展区 + 纯汉字应判为 HKSCS")
	}
	if hkscsLike([]byte{0x9D, 0xF5}, "�") || hkscsLike([]byte{0xA4, 0xA4}, "中文") {
		t.Error("替换字符或非扩展区不应判为 HKSCS")
	}
	if enc := big5Encoding([]byte{0x9D, 0xF5}, "中文"); enc != EncBig5HKSCS {
		t.Errorf("扩展区归类 = %q", enc)
	}
}
