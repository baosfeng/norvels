package textcode

import "sort"

// Table 是一份字频码表：entries 按字频降序排列，**下标即码位上界**——
// 前 singlesCap 项走 1 字节，其余走 2 字节，表外的字符走 3 字节兜底。
//
// 码表本身不参与 Encode/Decode（那是内置表的事），它只负责「训练 → 序列化 → 内嵌」这条链路。
type Table struct {
	version int
	entries []rune
	bmp     []uint16        // BMP 码点 → 码位+1（0 = 未进表），编码时 O(1) 查表
	astral  map[rune]uint16 // 补充平面字符 → 码位+1（正常表里没几个，用 map 省 8 MB）
}

// Train 按字频生成码表：出现次数降序，同频按码点升序（保证同样的输入永远得到同样的表）。
// 取前 tableCap 项，更长的长尾不进表、编码时走 3 字节兜底。
//
// 非法项跳过而不是报错：count ≤ 0、代理区码点（U+D800–U+DFFF）、超出 U+10FFFF 的值
// 都来自脏语料统计，不该让训练失败。counts 为 nil 时返回空表（所有字符走兜底），照常可用。
//
// 排序只看字频，不看字符本身的 UTF-8 长度：中文语料里 ASCII（空白、标点、数字）频次
// 足以自然落在 1 字节层；若拿来做纯非 ASCII 语料，需自己确认 ASCII 没有掉进 2 字节层。
func Train(counts map[rune]int64) *Table {
	entries := make([]rune, 0, len(counts))
	for r, n := range counts {
		if n <= 0 || !validScalar(r) {
			continue
		}
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := counts[entries[i]], counts[entries[j]]
		if a != b {
			return a > b
		}
		return entries[i] < entries[j]
	})
	if len(entries) > tableCap {
		entries = entries[:tableCap]
	}
	return newTable(tableVersion, entries)
}

// newTable 用有序码点建表，同时建好编码侧的查表结构。
func newTable(version int, entries []rune) *Table {
	t := &Table{version: version, entries: entries, bmp: make([]uint16, 0x10000)}
	for i, r := range entries {
		code := uint16(i) + 1 // +1 让 0 表示「未进表」，省一次 bool 查表
		if uint32(r) > 0xFFFF {
			if t.astral == nil {
				t.astral = make(map[rune]uint16, 8)
			}
			t.astral[r] = code
			continue
		}
		if t.bmp[r] == 0 {
			t.bmp[r] = code
		}
	}
	return t
}

// code 查字符的码位；未进表返回 false（长尾交给 3 字节兜底，不是错误）。
func (t *Table) code(r rune) (int, bool) {
	if u := uint32(r); u <= 0xFFFF {
		if c := t.bmp[u]; c != 0 {
			return int(c) - 1, true
		}
		return 0, false
	}
	if c, ok := t.astral[r]; ok {
		return int(c) - 1, true
	}
	return 0, false
}

// at 返回码位对应的字符；码位超出本表项数时返回 false（说明流与表不匹配）。
func (t *Table) at(code int) (rune, bool) {
	if code < 0 || code >= len(t.entries) {
		return 0, false
	}
	return t.entries[code], true
}

// validScalar 报告 r 是否是 UTF-8 能表示的标量值（排除代理区与越界值）。
func validScalar(r rune) bool {
	u := uint32(r)
	return u <= 0x10FFFF && (u < 0xD800 || u > 0xDFFF)
}
