package store

import (
	"errors"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// 正文存储：**按块压缩**，不是按章压缩。
//
// 为什么不是按章：章节平均只有 13 KB，对压缩器来说样本太小。同一批真实语料实测
// （12 本 / 53.2 MB）：
//
//	按章独立 flate-6   49.0%
//	按章独立 zstd-best 48%
//	1 MB 块 zstd-best 34.1%   ← 体积几乎减半
//
// 原因：压缩器的字典窗口跨章节才能吃到「同一本书反复出现的词、人名、地名、句式」。
//
// 为什么也不是整本：整本 31.9%，只比 1 MB 块好 2.2 个百分点，但「读一章要解压整本」，
// 翻页从毫秒级掉到十几毫秒，内存峰值还随书本大小线性涨。1 MB 是拐点。
//
// 为什么不转 GB18030：UTF-8 → GB18030 单独就有 -33%，但实测 12 本里 1 本在往返后
// 有 9 个私用区字符（U+E0xx~U+E4xx）被改写成 U+F1xx~U+F4xx——这是网文站的反爬手法，
// GB18030 的私用区映射与站点自造编码冲突，转码即破坏正文。用户要的是「读章节内容」，
// 正文被悄悄改字不可接受，所以直接压 UTF-8。
//
// 存储格式：块 BLOB 的首字节是算法标记，其后是压缩后的整块字节。
//
//	0x00  未压缩（块太短时直接存原文）
//	0x01  flate-6（保留：早期骨架用过，读路径仍认它，便于混存的老块读出）
//	0x02  zstd-best（当前写入方）
//	其余  保留给将来的算法

// 块 BLOB 的算法标记（格式位的唯一来源，读写两侧都必须引用这里的常量）。
const (
	contentAlgoRaw   byte = 0x00 // 未压缩
	contentAlgoFlate byte = 0x01 // flate-6 raw deflate（历史格式，仅读）
	contentAlgoZstd  byte = 0x02 // zstd SpeedBestCompression（当前写入格式）
)

// BlockTargetBytes 是单个压缩块的目标原始大小（1 MB）。
//
// 块的边界按**章节边界**切，不按字节切：从某章开始往块里塞，塞到再加下一章就会
// 超过 1 MB（或本章自己就超过）为止。这样一块永远覆盖整数个章节，块内偏移直接
// 指向完整章节，不会切碎正文。
const BlockTargetBytes = 1 << 20

// contentRawThreshold 是「块不值得压缩」的下限（字节）。
// 只有极短的书（整本低于该值）才会命中，此时压缩反而可能变大。
const contentRawThreshold = 64

// ErrCorruptContent 表示正文字节无法按格式解开（标记未知或压缩流损坏）。
var ErrCorruptContent = errors.New("store: corrupt chapter content")

// 编码器/解码器池：zstd 的 Encoder 会分配较大的窗口缓冲，每块新建代价明显。
// store.SaveBook 是单写者串行，但测试与未来并发读仍要安全，故用 sync.Pool。
var (
	zstdEncoderPool = sync.Pool{New: func() any {
		enc, err := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedBestCompression),
			zstd.WithEncoderConcurrency(1),
		)
		if err != nil {
			// 只有参数非法才会失败；参数是编译期常量，走到这里说明代码被改坏了。
			panic("store: init zstd encoder: " + err.Error())
		}
		return enc
	}}
	zstdDecoderPool = sync.Pool{New: func() any {
		dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
		if err != nil {
			panic("store: init zstd decoder: " + err.Error())
		}
		return dec
	}}
)

// compressContent 把一个**块**的原始字节编码成待入库的 BLOB：算法标记 + 载荷。
//
// 契约要点（换成任何算法都必须保持）：
//   - 输入是一整块的原始字节，输出是这一块的压缩形态，不产生跨块状态；
//   - 空输入返回 nil（对应 SQL 的 NULL / 零长 BLOB）；
//   - 输出对同一输入必须稳定，便于测试与体积对账。
func compressContent(raw []byte) ([]byte, error) {
	if compressFailureHook.Load() {
		return nil, errors.New("store: compress failure injected by test hook")
	}
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) < contentRawThreshold {
		return append([]byte{contentAlgoRaw}, raw...), nil
	}

	enc, ok := zstdEncoderPool.Get().(*zstd.Encoder)
	if !ok {
		return nil, errors.New("store: zstd encoder pool type mismatch")
	}
	defer zstdEncoderPool.Put(enc)

	out := enc.EncodeAll(raw, make([]byte, 0, len(raw)/2+16))
	// 压缩后反而更大（高度随机/极短的输入）时退回原文，别让存储为负收益买单。
	if len(out) >= len(raw) {
		return append([]byte{contentAlgoRaw}, raw...), nil
	}
	packed := make([]byte, 0, len(out)+1)
	packed = append(packed, contentAlgoZstd)
	return append(packed, out...), nil
}

// decompressContent 把库里的一段块字节还原成**整块**的原始字节。
//
// 契约（key design constraint，换算法时不要破坏）：
//
//	输入是一块的完整字节，输出是这一块的原始内容；
//	解压过程**不依赖其他块的状态**，也不读任何外部上下文。
//
// 取单章正文由 chapterAt 在这个结果上切片完成。store 层不缓存解压结果：
// 一块解压约 1 ms，缓存收益低；需要连续读多章（预加载）时由调用方自行持有缓存。
func decompressContent(packed []byte) ([]byte, error) {
	if len(packed) == 0 {
		return nil, nil
	}
	switch packed[0] {
	case contentAlgoRaw:
		return packed[1:], nil
	case contentAlgoFlate:
		return inflateBlock(packed[1:])
	case contentAlgoZstd:
		return inflateZstdBlock(packed[1:])
	default:
		return nil, fmt.Errorf("%w: unknown content algorithm 0x%02x", ErrCorruptContent, packed[0])
	}
}

// inflateZstdBlock 用池化解码器解一块 zstd 数据。
func inflateZstdBlock(payload []byte) ([]byte, error) {
	dec, ok := zstdDecoderPool.Get().(*zstd.Decoder)
	if !ok {
		return nil, errors.New("store: zstd decoder pool type mismatch")
	}
	defer zstdDecoderPool.Put(dec)
	out, err := dec.DecodeAll(payload, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: zstd: %v", ErrCorruptContent, err)
	}
	return out, nil
}

// chapterAt 从**已解压的块**里切出一章的正文。
//
// off/len 来自 chapters 表的 block_off / block_len，单位是字节。
// 越界说明块与章节记录不一致（迁移错误或数据损坏），必须报错而不是返回半截正文——
// 静默截断会让用户读到缺字的章节却不自知。
func chapterAt(block []byte, off, length int) (string, error) {
	if length == 0 {
		return "", nil
	}
	if off < 0 || length < 0 || off > len(block) || off+length > len(block) {
		return "", fmt.Errorf("%w: span [%d,%d) out of block size %d",
			ErrCorruptContent, off, off+length, len(block))
	}
	return string(block[off : off+length]), nil
}

// splitBlocks 按**章节边界**把一章正文序列切成若干块，返回每块覆盖的章节下标区间。
//
// 规则：从当前章开始往块里塞，塞到「再加下一章就超过 BlockTargetBytes」为止。
// 单章自己就超过 1 MB 时独占一块（实测最长单章约 96 KB，这里只是兜底）。
//
// 返回值 blocks[i] = [first, last] 闭区间，与 chapters 的输入顺序一一对应。
func splitBlocks(sizes []int) [][2]int {
	if len(sizes) == 0 {
		return nil
	}
	blocks := make([][2]int, 0, len(sizes)/64+1)
	first, acc := 0, 0
	for i, n := range sizes {
		if acc > 0 && acc+n > BlockTargetBytes {
			blocks = append(blocks, [2]int{first, i - 1})
			first, acc = i, 0
		}
		acc += n
	}
	blocks = append(blocks, [2]int{first, len(sizes) - 1})
	return blocks
}

// joinChapters 把一段章节正文拼成块的原始字节，返回拼接结果与每章的块内偏移。
//
// 偏移以字节计，与 chapters.block_off / block_len 直接对应。
func joinChapters(contents []string) ([]byte, []int) {
	total := 0
	for _, c := range contents {
		total += len(c)
	}
	buf := make([]byte, 0, total)
	offs := make([]int, len(contents))
	for i, c := range contents {
		offs[i] = len(buf)
		buf = append(buf, c...)
	}
	return buf, offs
}
