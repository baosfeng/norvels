package main

import (
	"fmt"
	"sort"
	"unicode/utf8"
)

// bmpSize 是基本多文种的码位数（0x10000）。
const bmpSize = 0x10000

// counter 是单个 worker 的计数桶（合并在 countAll 里做）。
//
// 为什么基本平面用定长数组而不是 map：全库十几亿字符绝大多数落在基本平面，
// 数组下标直取比 map 查找快一个数量级；超出基本平面的长尾（含私用区）才走 map。
type counter struct {
	bmp      []uint64
	hi       map[rune]uint64
	bad      [256]uint64
	badTotal uint64
	runes    uint64
	rawBytes uint64 // 解压后的原始字节数（与 chapter_blocks.raw_bytes 对账用）
	blocks   uint64
	algo     [256]uint64
	err      error // 本 worker 遇到的第一处损坏块（不中断扫描，最后统一报）
}

func newCounter() *counter {
	return &counter{bmp: make([]uint64, bmpSize), hi: make(map[rune]uint64)}
}

// addBlock 记一块：先记算法标记，再解压、按 rune 统计。
func (c *counter) addBlock(b block) error {
	if len(b.packed) == 0 {
		return nil
	}
	c.blocks++
	c.algo[b.packed[0]]++
	raw, err := decompress(b.packed)
	if err != nil {
		return fmt.Errorf("块 #%d: %w", b.id, err)
	}
	c.rawBytes += uint64(len(raw))
	c.countRunes(raw)
	return nil
}

// countRunes 按 UTF-8 解码计数；非法字节按单字节单独记，**不计入** total_rune。
func (c *counter) countRunes(buf []byte) {
	for len(buf) > 0 {
		r, size := utf8.DecodeRune(buf)
		if r == utf8.RuneError && size == 1 {
			c.bad[buf[0]]++
			c.badTotal++
			buf = buf[1:]
			continue
		}
		if int(r) < bmpSize {
			c.bmp[r]++
		} else {
			c.hi[r]++
		}
		c.runes++
		buf = buf[size:]
	}
}

// merge 把另一个桶并进来。
func (c *counter) merge(o *counter) {
	for i, n := range o.bmp {
		if n != 0 {
			c.bmp[i] += n
		}
	}
	for r, n := range o.hi {
		c.hi[r] += n
	}
	for i, n := range o.bad {
		c.bad[i] += n
	}
	c.badTotal += o.badTotal
	c.runes += o.runes
	c.rawBytes += o.rawBytes
	c.blocks += o.blocks
	for i, n := range o.algo {
		c.algo[i] += n
	}
}

// entry 是一个码位及其出现次数。
type entry struct {
	r rune
	n uint64
}

// entries 返回按次数降序（同次数按码位升序，保证输出稳定）的完整列表。
func (c *counter) entries() []entry {
	out := make([]entry, 0, len(c.hi)+1024)
	for i, n := range c.bmp {
		if n != 0 {
			out = append(out, entry{r: rune(i), n: n})
		}
	}
	for r, n := range c.hi {
		if n != 0 {
			out = append(out, entry{r: r, n: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].r < out[j].r
	})
	return out
}

// algoNames 把 256 槽的标记直方图收敛成名读映射。
func (c *counter) algoNames() map[string]uint64 {
	out := make(map[string]uint64, 4)
	for i, n := range c.algo {
		if n != 0 {
			out[algoName(byte(i))] = n
		}
	}
	return out
}

func algoName(b byte) string {
	switch b {
	case algoRaw:
		return "raw"
	case algoFlate:
		return "flate"
	case algoZstd:
		return "zstd"
	default:
		return fmt.Sprintf("unknown:0x%02x", b)
	}
}

// badNames 把非法字节计数收敛成 "0xNN" → 次数。
func (c *counter) badNames() map[string]uint64 {
	out := make(map[string]uint64, 8)
	for i, n := range c.bad {
		if n != 0 {
			out[fmt.Sprintf("0x%02x", i)] = n
		}
	}
	return out
}
