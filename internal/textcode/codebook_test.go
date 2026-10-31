package textcode

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// TestBuiltinTable 钉住 v1 码表的关键事实：项数、前三个码位、CRC 自校验。
//
// 这张表由全库字频训练产出（频次数据与训练过程不入库，见包文档「码表」一节）：
// 前三个码位是全库频次前三的字符，项数等于布局容量——任何一项变了都说明 .bin 被换过。
func TestBuiltinTable(t *testing.T) {
	if Version() != tableVersion || builtin.version != tableVersion {
		t.Fatalf("版本 %d / %d，想要 %d", Version(), builtin.version, tableVersion)
	}
	if len(builtin.entries) != tableCap {
		t.Fatalf("内置码表 %d 项，想要布局容量 %d 项", len(builtin.entries), tableCap)
	}
	for i, want := range []rune{0xFF0C, 0x7684, 0x000A} { // ，的 换行
		if builtin.entries[i] != want {
			t.Fatalf("第 %d 项是 U+%04X，想要 U+%04X", i, builtin.entries[i], want)
		}
	}
}

// TestBuiltinBlobSelfCheck 确认 codebook_v1.bin 自描述且自校验：CRC32C 对得上、能独立加载。
// 别人只拿到这份字节也能解出来，不依赖本包的常量。
func TestBuiltinBlobSelfCheck(t *testing.T) {
	tail := len(builtinBlob) - trailerSize
	if got, want := binary.BigEndian.Uint32(builtinBlob[tail:]), crc32.Checksum(builtinBlob[:tail], castagnoli); got != want {
		t.Fatalf("CRC32C %08X != %08X", got, want)
	}
	loaded, err := LoadTable(builtinBlob)
	if err != nil {
		t.Fatalf("加载内置码表失败: %v", err)
	}
	if len(loaded.entries) != len(builtin.entries) {
		t.Fatalf("独立加载出 %d 项，进程内是 %d 项", len(loaded.entries), len(builtin.entries))
	}
	if want := headerSize + 4*len(builtin.entries) + trailerSize; len(builtinBlob) != want {
		t.Fatalf("内置码表 %d 字节，按项数应为 %d 字节", len(builtinBlob), want)
	}
}

// TestBuiltinTableEntries 校验码点合法且不重复（重复会让解码有歧义）。
func TestBuiltinTableEntries(t *testing.T) {
	seen := make(map[rune]struct{}, len(builtin.entries))
	for i, r := range builtin.entries {
		if !validScalar(r) {
			t.Fatalf("第 %d 项 U+%04X 不是合法字符", i, r)
		}
		if _, dup := seen[r]; dup {
			t.Fatalf("第 %d 项 U+%04X 重复", i, r)
		}
		seen[r] = struct{}{}
	}
}

// TestBuiltinHotPath 确认最高频字真的走 1 字节层（否则基准测的是兜底路径）。
func TestBuiltinHotPath(t *testing.T) {
	hot := builtin.entries[0]
	got, err := Encode(nil, []byte(string(hot)))
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("最高频字 %q 编码后 %d 字节，想要 1 字节", hot, len(got))
	}
}
