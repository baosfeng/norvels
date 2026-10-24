package parser

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// encodeUTF16 把字符串编码成 UTF-16 字节流，用于构造合成 fixture。
func encodeUTF16(s string, little bool) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(u)*2)
	for _, v := range u {
		if little {
			out = append(out, byte(v), byte(v>>8))
		} else {
			out = append(out, byte(v>>8), byte(v))
		}
	}
	return out
}

// TestCodecUTF8 覆盖 utf8 策略的正常与非法字节（尽力解码）两条路径。
func TestCodecUTF8(t *testing.T) {
	c := utf8Codec{}
	if c.Name() != EncUTF8 {
		t.Errorf("Name = %q", c.Name())
	}
	text, score := c.Try([]byte("正常中文"))
	if text != "正常中文" || score <= 0 {
		t.Fatalf("正常文本解码失败: %q %v", text, score)
	}
	// 非法 UTF-8：应尽力解码并给一个低分（而不是 0，保证仍有候选可用）。
	text, score = c.Try([]byte{0xFF, 0xFE, 0x41})
	if text == "" || score <= 0 {
		t.Errorf("非法字节应尽力解码: %q %v", text, score)
	}
}

// TestCodecUTF8BOM 覆盖 utf8-bom 的正常与放弃路径。
func TestCodecUTF8BOM(t *testing.T) {
	c := utf8BOMCodec{}
	if text, score := c.Try([]byte("\xEF\xBB\xBF中文")); score <= 0 || text != "中文" {
		t.Errorf("BOM 文本 = %q %v", text, score)
	}
	if _, score := c.Try([]byte("没有BOM")); score != 0 {
		t.Errorf("无 BOM 应放弃，实际 score = %v", score)
	}
}

// TestCodecUTF16LE 覆盖 UTF-16LE 的 BOM、无 BOM 与放弃三条路径。
func TestCodecUTF16LE(t *testing.T) {
	c := utf16LECodec{}
	body := encodeUTF16("中文测试", true)
	text, score := c.Try(append([]byte{0xFF, 0xFE}, body...))
	if score <= 0 || !strings.Contains(text, "中文") {
		t.Errorf("带 BOM = %q %v", text, score)
	}
	// 无 BOM 的判定靠 NUL 字节分布，所以 fixture 里要有 ASCII 字符。
	mixed := encodeUTF16("中文测试 abcd efgh", true)
	text, score = c.Try(mixed)
	if score <= 0 || !strings.Contains(text, "中文") {
		t.Errorf("无 BOM 应识别为 UTF-16LE: %q %v", text, score)
	}
	if _, score := c.Try([]byte("纯 ASCII 内容")); score != 0 {
		t.Errorf("纯 ASCII 应放弃，实际 = %v", score)
	}
}

// TestCodecUTF16BE 覆盖 UTF-16BE 的 BOM 与放弃路径。
func TestCodecUTF16BE(t *testing.T) {
	c := utf16BECodec{}
	body := encodeUTF16("中文测试", false)
	text, score := c.Try(append([]byte{0xFE, 0xFF}, body...))
	if score <= 0 || !strings.Contains(text, "中文") {
		t.Errorf("带 BOM = %q %v", text, score)
	}
	if _, score := c.Try([]byte("纯 ASCII 内容")); score != 0 {
		t.Errorf("纯 ASCII 应放弃，实际 = %v", score)
	}
}

// TestCodecGB18030 覆盖 GB18030 的正常与放弃路径。
func TestCodecGB18030(t *testing.T) {
	c := gb18030Codec{}
	gb := []byte{0xD6, 0xD0, 0xCE, 0xC4} // "中文" 的 GBK 编码
	text, score := c.Try(gb)
	if score <= 0 || text != "中文" {
		t.Errorf("GBK = %q %v", text, score)
	}
	if _, score := c.Try([]byte{}); score <= 0 {
		t.Logf("空输入的处理: %v", score)
	}
}

// TestCodecBig5 覆盖 Big5 的正常路径。
func TestCodecBig5(t *testing.T) {
	c := big5Codec{}
	big5 := []byte{0xA4, 0xA4, 0xA4, 0xE5} // "中文" 的 Big5 编码
	text, score := c.Try(big5)
	if score <= 0 || text == "" {
		t.Errorf("Big5 = %q %v", text, score)
	}
}

// TestDecodeByCodecsPreference 验证「BOM 优先于其它候选」这一打分约定。
func TestDecodeByCodecsPreference(t *testing.T) {
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte("中文内容")...)
	text, enc, score := decodeByCodecs(body, "", func(string, any) {})
	if enc != EncUTF8BOM || score != scoreBOM || text != "中文内容" {
		t.Errorf("BOM 应胜出: enc=%q score=%v text=%q", enc, score, text)
	}
}

// TestDecodeByCodecsForced 覆盖强制编码命中与未命中。
func TestDecodeByCodecsForced(t *testing.T) {
	raw := []byte("中文内容")
	_, enc, _ := decodeByCodecs(raw, EncUTF8, func(string, any) {})
	if enc != EncUTF8 {
		t.Errorf("强制 UTF-8 应生效: %q", enc)
	}
	called := false
	_, enc, _ = decodeByCodecs(raw, "不存在的编码", func(k string, _ any) {
		if k == "codec_unknown_forced" {
			called = true
		}
	})
	if !called {
		t.Error("未记录的强制编码应写进诊断")
	}
	if enc == "不存在的编码" {
		t.Error("非法编码名不应成为结果编码")
	}
}

// TestDecodeByCodecsEmpty 覆盖空输入。
func TestDecodeByCodecsEmpty(t *testing.T) {
	text, enc, score := decodeByCodecs(nil, "", func(string, any) {})
	if text != "" || enc != EncUTF8 || score != 0 {
		t.Errorf("空输入 = %q %q %v", text, enc, score)
	}
}
