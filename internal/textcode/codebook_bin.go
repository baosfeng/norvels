package textcode

import (
	"encoding/binary"
	"hash/crc32"
)

// 码表二进制的线格式（大端）：
//
//	[0:4]        魔数 "NVTC"
//	[4:6]        码表版本（v1 = 1），与 Version() 一致
//	[6:8]        1 字节层容量（v1 = 192）；与编译进来的布局不符即拒绝加载
//	[8:12]       码点数 n（按字频降序，下标即码位）
//	[12:16]      保留位（0），留给将来的布局参数
//	[16:16+4n)   n × uint32 码点
//	[末尾 4)      CRC32C（Castagnoli），覆盖前面全部字节
//
// 自描述的意义：把这段字节单独交出去（换工具、换语言、换机器）也能照着解出来，
// 不依赖本包的常量；版本与布局不符时宁可拒载，也不猜着一半对一半错地解。
const (
	magic       = "NVTC"
	headerSize  = 16
	trailerSize = 4
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// MarshalBinary 把码表序列化成自描述的一整块字节（格式见本文件顶部注释）。
func (t *Table) MarshalBinary() ([]byte, error) {
	if t == nil {
		return nil, tablef("nil 码表无法序列化")
	}
	if t.version != tableVersion {
		return nil, tablef("码表版本 %d 与本程序支持的 %d 不符", t.version, tableVersion)
	}
	if len(t.entries) > tableCap {
		return nil, tablef("码表 %d 项超过布局上限 %d", len(t.entries), tableCap)
	}
	out := make([]byte, 0, headerSize+4*len(t.entries)+trailerSize)
	out = append(out, magic...)
	out = binary.BigEndian.AppendUint16(out, uint16(t.version))
	out = binary.BigEndian.AppendUint16(out, uint16(singlesCap))
	out = binary.BigEndian.AppendUint32(out, uint32(len(t.entries)))
	out = binary.BigEndian.AppendUint32(out, 0) // 保留位
	for _, r := range t.entries {
		out = binary.BigEndian.AppendUint32(out, uint32(r))
	}
	return binary.BigEndian.AppendUint32(out, crc32.Checksum(out, castagnoli)), nil
}

// LoadTable 解析 MarshalBinary 产出的字节。任何不自洽的输入（魔数、版本、布局、
// 项数、长度、CRC、码点非法或重复）都返回 ErrInvalidTable，不做部分加载。
func LoadTable(b []byte) (*Table, error) {
	body, n, err := parseHeader(b)
	if err != nil {
		return nil, err
	}
	entries, err := parseEntries(body, n)
	if err != nil {
		return nil, err
	}
	return newTable(tableVersion, entries), nil
}

// parseHeader 校验头部与 CRC，返回码点区与项数。
func parseHeader(b []byte) ([]byte, int, error) {
	if len(b) < headerSize+trailerSize {
		return nil, 0, tablef("长度 %d 小于最小码表长度 %d", len(b), headerSize+trailerSize)
	}
	if string(b[:4]) != magic {
		return nil, 0, tablef("魔数 %q 不是 %q", b[:4], magic)
	}
	if v := int(binary.BigEndian.Uint16(b[4:])); v != tableVersion {
		return nil, 0, tablef("码表版本 %d 与本程序支持的 %d 不符", v, tableVersion)
	}
	if s := int(binary.BigEndian.Uint16(b[6:])); s != singlesCap {
		return nil, 0, tablef("1 字节层容量 %d 与布局 %d 不符", s, singlesCap)
	}
	n := int(binary.BigEndian.Uint32(b[8:]))
	if n > tableCap {
		return nil, 0, tablef("码点数 %d 超过布局上限 %d", n, tableCap)
	}
	if want := headerSize + 4*n + trailerSize; len(b) != want {
		return nil, 0, tablef("长度 %d 与码点数 %d 对不上（应为 %d）", len(b), n, want)
	}
	if sum := binary.BigEndian.Uint32(b[len(b)-trailerSize:]); sum != crc32.Checksum(b[:len(b)-trailerSize], castagnoli) {
		return nil, 0, tablef("CRC32C 校验失败（尾部 0x%08X）", sum)
	}
	return b[headerSize : len(b)-trailerSize], n, nil
}

// parseEntries 读出码点并校验：必须是合法标量值，且不能重复（重复会让解码有歧义）。
func parseEntries(body []byte, n int) ([]rune, error) {
	entries := make([]rune, n)
	seen := make(map[rune]struct{}, n)
	for i := range entries {
		u := binary.BigEndian.Uint32(body[4*i:])
		r := rune(u)
		if u > 0x10FFFF || !validScalar(r) {
			return nil, tablef("第 %d 个码点 U+%04X 不是合法字符", i, u)
		}
		if _, dup := seen[r]; dup {
			return nil, tablef("第 %d 个码点 U+%04X 重复", i, u)
		}
		seen[r] = struct{}{}
		entries[i] = r
	}
	return entries, nil
}
