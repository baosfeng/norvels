package textcode

import (
	"bytes"
	"errors"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestRoundTrip 是这一层最基本的承诺：任意字节序列，编码再解码逐字节相同。
// 覆盖中文、私用区、4 字节 emoji、非法 UTF-8 与空串。
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"空串", nil},
		{"纯 ASCII", []byte("hello, world\r\n\t")},
		{"中文正文", []byte("第一章 风起于青萍之末，浪成于微澜之间。")},
		{"私用区", []byte("\uE000\uE001\uE002\uE0FF\uF8FF")},
		{"4 字节 emoji", []byte("\U0001F600\U0001F680\U0001F600")},
		{"中英混排", []byte("abc 中文 \uE000 \U0001F600 结束")},
		{"单个非法字节", []byte{0x80}},
		{"截断的 3 字节序列", []byte{0xE5, 0x8F}},
		{"超长编码", []byte{0xC0, 0x80}},
		{"代理区编码", []byte{0xED, 0xA0, 0x80}},
		{"连续非法字节", bytes.Repeat([]byte{0xFF, 0xFE}, 200)},
		{"非法字节夹在正文里", append(append([]byte("正文"), 0x80, 0xFF, 'a'), []byte("续")...)},
		{"BMP 边界码点", []byte("\u0000\u007F\u0080\u07FF\u0800\uFFEF\uFFFD\uFFFE\uFFFF")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc, err := Encode(nil, c.in)
			if err != nil {
				t.Fatalf("Encode 失败: %v", err)
			}
			if c.name == "空串" && len(enc) != 0 {
				t.Fatalf("空输入编码出 %d 字节，想要 0", len(enc))
			}
			got, err := Decode(nil, enc)
			if err != nil {
				t.Fatalf("Decode 失败: %v", err)
			}
			if !bytes.Equal(got, c.in) {
				t.Fatalf("往返不一致：\n in = %x\nout = %x", c.in, got)
			}
		})
	}
}

// TestRoundTripAllScalars 遍历全部合法码点，证明「任意 Unicode 都能编码」不是抽样结论。
func TestRoundTripAllScalars(t *testing.T) {
	if testing.Short() {
		t.Skip("全码点往返在 -short 下跳过")
	}
	var src []byte
	for u := rune(0); u <= 0x10FFFF; u++ {
		if u >= 0xD800 && u <= 0xDFFF {
			continue
		}
		src = utf8.AppendRune(src, u)
	}
	enc, err := Encode(nil, src)
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	if len(enc) > len(src) {
		t.Fatalf("编码后 %d 字节 > 原始 %d 字节：三字节层不该比 UTF-8 更大", len(enc), len(src))
	}
	got, err := Decode(nil, enc)
	if err != nil {
		t.Fatalf("Decode 失败: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("全码点往返不一致")
	}
}

// TestEncodeLayout 钉住四个字节层各自的字节形态——布局是跨版本契约，不能悄悄变。
func TestEncodeLayout(t *testing.T) {
	hot, ok := builtin.at(0)
	if !ok {
		t.Fatal("码表为空")
	}
	if enc := mustEncode(t, string(hot)); !bytes.Equal(enc, []byte{0x00}) {
		t.Fatalf("1 字节层首项编码成 %x，想要 00", enc)
	}
	if len(builtin.entries) <= singlesCap {
		t.Fatal("占位表太短，没有 2 字节层可测")
	}
	cold, _ := builtin.at(singlesCap)
	if enc := mustEncode(t, string(cold)); !bytes.Equal(enc, []byte{0xC0, 0x00}) {
		t.Fatalf("2 字节层首项编码成 %x，想要 c000", enc)
	}
	// 码表外的字符：3 字节层直接是码点（U+4E01 → E0 4E 01，不是 UTF-8 的 E4 B8 81）。
	orphan := firstUncoded(t, 0x4E01)
	if enc := mustEncode(t, string(orphan)); !bytes.Equal(enc, []byte{0xE0, 0x4E, 0x01}) {
		t.Fatalf("兜底字符编码成 %x，想要 e04e01", enc)
	}
	// 非法字节：转义段 = 0xFF + 段长 + 原始字节。
	if enc := mustEncode(t, "\x80\xFF"); !bytes.Equal(enc, []byte{0xFF, 0x02, 0x80, 0xFF}) {
		t.Fatalf("非法字节编码成 %x，想要 ff0280ff", enc)
	}
	// 连续非法字节超过 255 个时切成多段（段长字段只有一个字节）。
	if enc := mustEncode(t, strings.Repeat("\x80", 300)); len(enc) != 2+255+2+45 {
		t.Fatalf("300 个非法字节编码成 %d 字节，想要 304（两段转义）", len(enc))
	}
}

// TestEncodeAppendPreservesPrefix 确认「追加」语义：dst 里原有内容必须原样保留。
func TestEncodeAppendPreservesPrefix(t *testing.T) {
	prefix := []byte("前缀")
	dst := append([]byte(nil), prefix...)
	enc, err := Encode(dst, []byte("正文"))
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	if !bytes.HasPrefix(enc, prefix) {
		t.Fatalf("Encode 覆盖了 dst 前缀：%x", enc)
	}
	out := append([]byte(nil), prefix...)
	dec, err := Decode(out, enc[len(prefix):])
	if err != nil {
		t.Fatalf("Decode 失败: %v", err)
	}
	if !bytes.Equal(dec, append(prefix, []byte("正文")...)) {
		t.Fatalf("Decode 丢弃了 dst 前缀：%x", dec)
	}
}

// TestDecodeCorrupt 确认坏流一律报 ErrCorrupt，而不是解出半截正文。
func TestDecodeCorrupt(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"2 字节码截断", []byte{0xC1}},
		{"2 字节码越界", []byte{0xDF, 0xFF}},
		{"3 字节码截断", []byte{0xE5, 0x00}},
		{"3 字节码超出 U+10FFFF", []byte{0xF5, 0x00, 0x00}},
		{"3 字节码落在代理区", []byte{0xE0, 0xD8, 0x00}},
		{"转义段缺长度", []byte{0xFF}},
		{"转义段段长为 0", []byte{0xFF, 0x00}},
		{"转义段被截断", []byte{0xFF, 0x02, 0x41}},
		{"好前缀后跟坏码", []byte{0x00, 0xC1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode(nil, c.in)
			if err == nil {
				t.Fatalf("想要报错，实际解出 %x", got)
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("错误不是 ErrCorrupt：%v", err)
			}
		})
	}
}

// TestEmptyAndNilTable 覆盖两条「非正常路径」：空输入不分配，nil 码表报 ErrInvalidTable。
func TestEmptyAndNilTable(t *testing.T) {
	if enc, err := Encode(nil, nil); err != nil || len(enc) != 0 {
		t.Fatalf("Encode(nil, nil) = %x, %v", enc, err)
	}
	if dec, err := Decode(nil, nil); err != nil || len(dec) != 0 {
		t.Fatalf("Decode(nil, nil) = %x, %v", dec, err)
	}
	if _, err := encodeTable(nil, nil, []byte("a")); !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("nil 码表编码错误 = %v，想要 ErrInvalidTable", err)
	}
	if _, err := decodeTable(nil, nil, []byte{0x00}); !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("nil 码表解码错误 = %v，想要 ErrInvalidTable", err)
	}
}

// TestRandomBytesNeverPanic 随机脏字节反复往返：库里有真实脏字节（check-content 的 BadUTF8），
// 编码层必须照单全收，既不 panic 也不改写。
func TestRandomBytesNeverPanic(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		src := make([]byte, rnd.Intn(512))
		for i := range src {
			src[i] = byte(rnd.Intn(256))
		}
		// 混入一些合法中文，避免只测到纯随机噪声。
		src = append(src, []byte(strings.Repeat("正文", 3))...)
		enc, err := Encode(nil, src)
		if err != nil {
			t.Fatalf("Encode 失败: %v", err)
		}
		got, err := Decode(nil, enc)
		if err != nil {
			t.Fatalf("Decode 失败（%x）: %v", enc, err)
		}
		if !bytes.Equal(got, src) {
			t.Fatalf("随机往返不一致：\n in = %x\nout = %x", src, got)
		}
	}
}

// firstUncoded 从 start 起找一个不在内置码表里的字符，用来测 3 字节兜底层。
func firstUncoded(t *testing.T, start rune) rune {
	t.Helper()
	for r := start; r < start+0x1000; r++ {
		if _, ok := builtin.code(r); !ok {
			return r
		}
	}
	t.Fatal("找不到码表外的字符，占位表太大，测试前提不成立")
	return 0
}

func mustEncode(t *testing.T, s string) []byte {
	t.Helper()
	enc, err := Encode(nil, []byte(s))
	if err != nil {
		t.Fatalf("Encode(%q) 失败: %v", s, err)
	}
	return enc
}

// FuzzRoundTrip：任何字节序列都必须逐字节往返（go test -fuzz=FuzzRoundTrip 可加压）。
func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("中文abc"))
	f.Add([]byte{0xFF, 0x80, 0x00, 0xC0})
	f.Fuzz(func(t *testing.T, src []byte) {
		enc, err := Encode(nil, src)
		if err != nil {
			t.Fatalf("Encode 失败: %v", err)
		}
		got, err := Decode(nil, enc)
		if err != nil {
			t.Fatalf("Decode 失败: %v", err)
		}
		if !bytes.Equal(got, src) {
			t.Fatalf("往返不一致：\n in = %x\nout = %x", src, got)
		}
	})
}
