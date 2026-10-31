package store

import (
	"errors"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"

	"norvels/internal/textcode"
)

// 正文存储：**按块压缩**，不是按章压缩。
//
// 为什么不是按章：章节平均只有 13 KB，对压缩器来说样本太小，按章独立压缩实测 48%–49%。
// 原因：压缩器的字典窗口跨章节才能吃到「同一本书反复出现的词、人名、地名、句式」。
//
// 为什么是 8 MB 块：NAS 真实库 200 本抽样实测（同一批语料、同一口径）：
//
//	1 MB 块 纯 zstd-best        33.42%
//	1 MB 块 码本 + zstd-best    28.08%
//	8 MB 块 码本 + zstd-best    26.45%   ← 当前写入格式（解压 P50 32.5 ms / P95 37.6 ms）
//	整本     码本 + zstd-best    25.98%   （解压 P95 169 ms，最大书内存 386 MB）
//
// 整本相对 8 MB 只再省 0.47 个百分点，却让 P95 慢 4.5 倍、内存高 15 倍——「读一章先解压整本」
// 把翻页拖到百毫秒级，内存峰值还随书本大小线性涨。8 MB 是拐点。
//
// 为什么先过码本再压：UTF-8 给每个汉字固定 3 字节，而全库字频前 1000 覆盖 89.3%。textcode
// 按字频把正文**无损**重编码成平均 1.368 字节/字的码本流（UTF-8 的 45.6%），再交给 zstd，
// 才能贴着「1 阶上下文」的下限。重编码可逆：私用区字符、emoji、非法 UTF-8 字节都有出路
// （非法字节封进转义段原样保留），Decode(Encode(x)) 与 x 逐字节相同。
//
// 历史教训：早年试过 UTF-8 → GB18030 转码（单独就有 -33%），实测 12 本里 1 本有 9 个私用区
// 字符（U+E0xx～U+E4xx）被改写成 U+F1xx～U+F4xx——站点自造的反爬编码与 GB18030 的私用区
// 映射冲突。会改字的方案一律不能用，这也是现在选「无损码本」而不是「直接转码」的原因。
//
// 存储格式：块 BLOB 的首字节是算法标记，其后是压缩后的整块字节。
//
//	0x00  未压缩（块太短时直接存原文）
//	0x01  flate-6（保留：早期骨架用过，读路径仍认它，便于混存的老块读出）
//	0x02  zstd-best（码本之前的格式；码本对非中文正文无收益时也回退到它）
//	0x03  zstd( textcode.Encode(UTF-8) )（当前主写入格式）
//	其余  保留给将来的算法

// 块 BLOB 的算法标记（格式位的唯一来源，读写两侧都必须引用这里的常量）。
const (
	contentAlgoRaw      byte = 0x00 // 未压缩
	contentAlgoFlate    byte = 0x01 // flate-6 raw deflate（历史格式，仅读）
	contentAlgoZstd     byte = 0x02 // zstd SpeedBestCompression（码本之前的格式；码本无收益时的回退）
	contentAlgoCodebook byte = 0x03 // zstd SpeedBestCompression 压在 textcode.Encode 的码本流上
)

// BlockTargetBytes 是单个压缩块的目标原始大小（8 MB）。
//
// 块的边界按**章节边界**切，不按字节切：从某章开始往块里塞，塞到再加下一章就会
// 超过 8 MB（或本章自己就超过）为止。这样一块永远覆盖整数个章节，块内偏移直接
// 指向完整章节，不会切碎正文。
const BlockTargetBytes = 8 << 20

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
// 主写入格式是 0x03：先把 UTF-8 重编码成码本流（平均 1.368 字节/字），再 zstd-best。
// 两步都无损，往返逐字节相同。码本流不短于原文时（英文 / 纯 ASCII 正文）回退 0x02，
// 直接压 UTF-8。
//
// 契约要点（换成任何算法都必须保持）：
//   - 输入是一整块的原始字节，输出是这一块的压缩形态，不产生跨块状态；
//   - 空输入返回 nil（对应 SQL 的 NULL / 零长 BLOB）；
//   - 输出对同一输入必须稳定，便于测试与体积对账；
//   - 压缩后如果比原文还大，一律退回 0x00 原文，存储不为「负收益」买单。
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

	encoded, err := textcode.Encode(nil, raw)
	if err != nil {
		return nil, fmt.Errorf("store: textcode encode: %w", err)
	}
	// 码本对非中文正文没有收益甚至有害：ASCII 会落进 2 字节层，整段码本流比原文还长
	// （实测英文正文 ≈200%），再压也不如直接压 UTF-8（20.9% vs 13.9%）。长度判断是
	// 零成本的，命中就直接回退 0x02，绝不为了「统一格式」让英文书变大。
	algo, payload := contentAlgoCodebook, encoded
	if len(encoded) >= len(raw) {
		algo, payload = contentAlgoZstd, raw
	}
	out, err := zstdCompressBest(payload)
	if err != nil {
		return nil, err
	}
	if len(out) >= len(raw) {
		return append([]byte{contentAlgoRaw}, raw...), nil
	}
	packed := make([]byte, 0, len(out)+1)
	packed = append(packed, algo)
	return append(packed, out...), nil
}

// zstdCompressBest 用池化编码器压一段字节（SpeedBestCompression，单并发）。
func zstdCompressBest(raw []byte) ([]byte, error) {
	enc, ok := zstdEncoderPool.Get().(*zstd.Encoder)
	if !ok {
		return nil, errors.New("store: zstd encoder pool type mismatch")
	}
	defer zstdEncoderPool.Put(enc)
	return enc.EncodeAll(raw, make([]byte, 0, len(raw)/2+16)), nil
}

// decompressContent 把库里的一段块字节还原成**整块**的原始字节。
//
// 契约（key design constraint，换算法时不要破坏）：
//
//	输入是一块的完整字节，输出是这一块的原始内容；
//	解压过程**不依赖其他块的状态**，也不读任何外部上下文。
//
// 四个标记的读路径都必须保留：老库混存着 0x00/0x01/0x02 的块，写侧换算法不能把
// 已有正文读不出来。取单章正文由 chapterAt 在这个结果上切片完成。store 层不缓存
// 解压结果（让 Store 无状态）：连续读同一块的多章由调用方自己持有缓存。
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
	case contentAlgoCodebook:
		return inflateCodebookBlock(packed[1:])
	default:
		return nil, fmt.Errorf("%w: unknown content algorithm 0x%02x", ErrCorruptContent, packed[0])
	}
}

// inflateCodebookBlock 解一块 0x03：先 zstd，再把码本流还原成 UTF-8。
func inflateCodebookBlock(payload []byte) ([]byte, error) {
	encoded, err := inflateZstdBlock(payload)
	if err != nil {
		return nil, err
	}
	out, err := textcode.Decode(nil, encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: textcode: %v", ErrCorruptContent, err)
	}
	return out, nil
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
// 单章自己就超过 8 MB 时独占一块（实测最长单章约 96 KB，这里只是兜底）。
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
