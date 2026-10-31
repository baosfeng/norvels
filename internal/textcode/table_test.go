package textcode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

// TestLayoutConstants 钉住字节空间的划分：改这里等于改格式，必须同时改版本号。
func TestLayoutConstants(t *testing.T) {
	if singlesCap != 192 || doublesCap != 8192 || tableCap != 8384 {
		t.Fatalf("布局常量变了：singles=%d doubles=%d cap=%d", singlesCap, doublesCap, tableCap)
	}
	if byte(lead2Start) != 0xC0 || byte(lead3Start) != 0xE0 || byte(escapeByte) != 0xFF {
		t.Fatalf("分层字节变了：2 字节层 %d、3 字节层 %d、转义 %d", lead2Start, lead3Start, escapeByte)
	}
	if singlesCap+doublesCap != tableCap {
		t.Fatal("码表容量与两层容量对不上")
	}
}

// TestTrainOrdering 字号排序：次数降序，同频按码点升序（同样的字频永远训练出同样的表）。
func TestTrainOrdering(t *testing.T) {
	tbl := Train(map[rune]int64{'a': 1, 'b': 3, 'c': 3, 'd': 2})
	want := []rune{'b', 'c', 'd', 'a'}
	if len(tbl.entries) != len(want) {
		t.Fatalf("训练出 %d 项，想要 %d 项", len(tbl.entries), len(want))
	}
	for i, r := range want {
		if tbl.entries[i] != r {
			t.Fatalf("第 %d 项是 %q，想要 %q（整表 %q）", i, tbl.entries[i], r, tbl.entries)
		}
	}
}

// TestTrainTruncates 超出布局容量的长尾不进表（编码时走 3 字节兜底），表长被截到上限。
func TestTrainTruncates(t *testing.T) {
	counts := make(map[rune]int64, tableCap+600)
	for i := 0; i < tableCap+600; i++ {
		counts[rune(0x4E00+i)] = int64(tableCap + 600 - i)
	}
	tbl := Train(counts)
	if len(tbl.entries) != tableCap {
		t.Fatalf("训练出 %d 项，想要上限 %d 项", len(tbl.entries), tableCap)
	}
	if tbl.entries[0] != 0x4E00 {
		t.Fatalf("最高频字是 U+%04X，想要 U+4E00", tbl.entries[0])
	}
	if _, ok := tbl.code(rune(0x4E00 + tableCap + 1)); ok {
		t.Fatal("被截掉的长尾字符不该有码位")
	}
}

// TestTrainSkipsInvalid 脏统计不该让训练失败：非法码点与非正次数直接跳过。
func TestTrainSkipsInvalid(t *testing.T) {
	tbl := Train(map[rune]int64{
		'中':      10,
		0xD800:   9, // 代理区，UTF-8 里不存在
		0x110000: 8, // 超出 Unicode 上限
		'a':      0,
		'b':      -1,
		0x1F600:  7, // 补充平面：走 astral map
	})
	want := []rune{'中', 0x1F600}
	if len(tbl.entries) != len(want) {
		t.Fatalf("训练出 %d 项 %q，想要 %q", len(tbl.entries), tbl.entries, want)
	}
	if tbl.astral[0x1F600] != 2 {
		t.Fatalf("补充平面字符的码位是 %d，想要 2（码位+1）", tbl.astral[0x1F600])
	}
}

// TestTrainNilAndEmptyTable 空统计也能训练出一张能用的表：所有字符走 3 字节兜底。
func TestTrainNilAndEmptyTable(t *testing.T) {
	tbl := Train(nil)
	if tbl == nil || len(tbl.entries) != 0 {
		t.Fatalf("Train(nil) = %+v，想要空表", tbl)
	}
	src := []byte("正文 abc")
	enc, err := encodeTable(tbl, nil, src)
	if err != nil {
		t.Fatalf("空表编码失败: %v", err)
	}
	got, err := decodeTable(tbl, nil, enc)
	if err != nil {
		t.Fatalf("空表解码失败: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Fatalf("空表往返不一致：%q vs %q", got, src)
	}
}

// TestTrainedTableEncoding 用训练出来的表（含补充平面项）验证编码/解码两条查表路径。
func TestTrainedTableEncoding(t *testing.T) {
	tbl := Train(map[rune]int64{'中': 5, '文': 4, 0x1F600: 3})
	if enc, err := encodeTable(tbl, nil, []byte("中")); err != nil || len(enc) != 1 {
		t.Fatalf("最高频字编码 = %x, %v（想要 1 字节）", enc, err)
	}
	enc, err := encodeTable(tbl, nil, []byte("😀"))
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	if len(enc) != 1 {
		t.Fatalf("表内补充平面字符编码成 %x（想要 1 字节，说明 astral 查表没生效）", enc)
	}
	dec, err := decodeTable(tbl, nil, enc)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if string(dec) != "😀" {
		t.Fatalf("解码 = %q，想要 😀", dec)
	}
}

// rawBlob 手工拼一份码表字节（可注入非法内容），用来覆盖加载侧的校验分支。
func rawBlob(version, singles uint16, runes []uint32) []byte {
	out := make([]byte, 0, headerSize+4*len(runes)+trailerSize)
	out = append(out, magic...)
	out = binary.BigEndian.AppendUint16(out, version)
	out = binary.BigEndian.AppendUint16(out, singles)
	out = binary.BigEndian.AppendUint32(out, uint32(len(runes)))
	out = binary.BigEndian.AppendUint32(out, 0)
	for _, r := range runes {
		out = binary.BigEndian.AppendUint32(out, r)
	}
	return binary.BigEndian.AppendUint32(out, crc32.Checksum(out, castagnoli))
}

// TestMarshalLoadRoundTrip 序列化 → 反序列化必须完全还原，且再序列化字节不变。
func TestMarshalLoadRoundTrip(t *testing.T) {
	tbl := Train(map[rune]int64{'中': 5, '文': 4, 'a': 3, 0x1F600: 2})
	blob, err := tbl.MarshalBinary()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	loaded, err := LoadTable(blob)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(loaded.entries) != len(tbl.entries) {
		t.Fatalf("项数 %d != %d", len(loaded.entries), len(tbl.entries))
	}
	for i := range tbl.entries {
		if loaded.entries[i] != tbl.entries[i] {
			t.Fatalf("第 %d 项 %q != %q", i, loaded.entries[i], tbl.entries[i])
		}
	}
	again, err := loaded.MarshalBinary()
	if err != nil {
		t.Fatalf("再序列化失败: %v", err)
	}
	if !bytes.Equal(blob, again) {
		t.Fatal("同一张表两次序列化字节不同")
	}
}

// TestMarshalNilTable nil 码表不能序列化（否则会解引用空指针）。
func TestMarshalNilTable(t *testing.T) {
	var tbl *Table
	if _, err := tbl.MarshalBinary(); !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("nil 码表序列化错误 = %v，想要 ErrInvalidTable", err)
	}
	bad := &Table{version: tableVersion + 1}
	if _, err := bad.MarshalBinary(); !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("版本不符的序列化错误 = %v，想要 ErrInvalidTable", err)
	}
	tooLong := &Table{version: tableVersion, entries: make([]rune, tableCap+1)}
	if _, err := tooLong.MarshalBinary(); !errors.Is(err, ErrInvalidTable) {
		t.Fatalf("超长码表序列化错误 = %v，想要 ErrInvalidTable", err)
	}
}

// TestLoadTableRejects 加载侧 fail-closed：魔数、版本、布局、项数、长度、CRC、码点任一不自洽都拒载。
func TestLoadTableRejects(t *testing.T) {
	good := rawBlob(tableVersion, singlesCap, []uint32{'a', '中'})
	if _, err := LoadTable(good); err != nil {
		t.Fatalf("合法码表被拒: %v", err)
	}
	badMagic := append([]byte(nil), good...)
	badMagic[0] = 'X'
	badCRC := append([]byte(nil), good...)
	badCRC[len(badCRC)-1] ^= 0xFF
	countOver := append([]byte(nil), good...)
	binary.BigEndian.PutUint32(countOver[8:], tableCap+1)
	cases := []struct {
		name string
		in   []byte
	}{
		{"过短", []byte(magic)},
		{"魔数错", badMagic},
		{"版本错", rawBlob(tableVersion+1, singlesCap, []uint32{'a'})},
		{"布局错", rawBlob(tableVersion, singlesCap-1, []uint32{'a'})},
		{"项数超上限", countOver},
		{"长度对不上", append(good, 0x00)},
		{"CRC 错", badCRC},
		{"码点越界", rawBlob(tableVersion, singlesCap, []uint32{0x110000})},
		{"代理区码点", rawBlob(tableVersion, singlesCap, []uint32{0xD800})},
		{"码点重复", rawBlob(tableVersion, singlesCap, []uint32{'a', 'a'})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := LoadTable(c.in); !errors.Is(err, ErrInvalidTable) {
				t.Fatalf("错误 = %v，想要 ErrInvalidTable", err)
			}
		})
	}
}

// 内置码表字节的自描述与自校验见 codebook_test.go 的 TestBuiltinBlobSelfCheck。
