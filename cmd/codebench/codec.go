package main

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"

	"norvels/internal/textcode"
)

// 库内块 BLOB 的算法标记，与 internal/store/compress.go 保持一致（只读侧必须认得）。
const (
	storedRaw   byte = 0x00 // 未压缩
	storedFlate byte = 0x01 // flate-6（历史格式，仅读）
	storedZstd  byte = 0x02 // zstd SpeedBestCompression（当前写入格式）
)

// 本工具自己的块格式标记：1 字节标记 + 载荷；码本组用 0x03 与纯 zstd 区分。
const (
	benchZstd     byte = 0x02
	benchZstdCode byte = 0x03
)

// blockTarget 是「1 MB 块」的目标字节数，与 store.BlockTargetBytes 同值。
const blockTarget = 1 << 20

// compressor 持有单线程的 zstd 编解码器。
//
// 参数刻意与 internal/store 完全一致（SpeedBestCompression + 并发 1），
// 否则压缩率与吞吐都对不上库内基线。
type compressor struct {
	enc *zstd.Encoder
	dec *zstd.Decoder
}

// newCompressor 建一对 zstd 编解码器。
func newCompressor() (*compressor, error) {
	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, fmt.Errorf("建 zstd 编码器: %w", err)
	}
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("建 zstd 解码器: %w", err)
	}
	return &compressor{enc: enc, dec: dec}, nil
}

// Close 释放编解码器占用的窗口缓冲（Decoder.Close 不返回值）。
func (c *compressor) Close() {
	_ = c.enc.Close()
	c.dec.Close()
}

// compress 把一段字节压成 zstd 流（EncodeAll 不返回错误，失败会 panic）。
func (c *compressor) compress(raw []byte) []byte {
	return c.enc.EncodeAll(raw, make([]byte, 0, len(raw)/2+16))
}

// decompress 还原一段 zstd 流。
func (c *compressor) decompress(payload []byte) ([]byte, error) {
	out, err := c.dec.DecodeAll(payload, nil)
	if err != nil {
		return nil, fmt.Errorf("zstd 解压: %w", err)
	}
	return out, nil
}

// packOne 把一块原始 UTF-8 字节压成自包含 BLOB：[1 字节标记][载荷]。
// useCode 为真时先过 textcode.Encode，再压码本流。
func packOne(c *compressor, raw []byte, useCode bool) ([]byte, error) {
	body, marker := raw, benchZstd
	if useCode {
		coded, err := textcode.Encode(nil, raw)
		if err != nil {
			return nil, fmt.Errorf("textcode 编码: %w", err)
		}
		body, marker = coded, benchZstdCode
	}
	z := c.compress(body)
	out := make([]byte, 0, len(z)+1)
	out = append(out, marker)
	return append(out, z...), nil
}

// unpackOne 是 packOne 的逆：先解压，码本流再 textcode.Decode 回 UTF-8。
func unpackOne(c *compressor, blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	raw, err := c.decompress(blob[1:])
	if err != nil {
		return nil, err
	}
	if blob[0] == benchZstdCode {
		return textcode.Decode(nil, raw)
	}
	return raw, nil
}

// decompressStored 还原库内的一块（标记 + 载荷）。只读路径，绝不写。
func decompressStored(c *compressor, packed []byte) ([]byte, error) {
	if len(packed) == 0 {
		return nil, nil
	}
	switch packed[0] {
	case storedRaw:
		return packed[1:], nil
	case storedFlate:
		r := flate.NewReader(bytes.NewReader(packed[1:]))
		defer r.Close() //nolint:errcheck // 只读流
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("flate 解压库内块: %w", err)
		}
		return out, nil
	case storedZstd:
		return c.decompress(packed[1:])
	default:
		return nil, fmt.Errorf("未知的库内算法标记 0x%02x", packed[0])
	}
}

// blockSpans 返回按章节边界切块的下标闭区间；target <= 0 表示整本一块。
//
// 规则与 store.splitBlocks 逐字一致：从当前章往块里塞，塞到「再加下一章就超过
// target」为止；单章自己就超过 target 时独占一块。
func blockSpans(sizes []int, target int) [][2]int {
	if len(sizes) == 0 {
		return nil
	}
	if target <= 0 {
		return [][2]int{{0, len(sizes) - 1}}
	}
	spans := make([][2]int, 0, len(sizes)/64+1)
	first, acc := 0, 0
	for i, n := range sizes {
		if acc > 0 && acc+n > target {
			spans = append(spans, [2]int{first, i - 1})
			first, acc = i, 0
		}
		acc += n
	}
	return append(spans, [2]int{first, len(sizes) - 1})
}

// offsetsOf 把每章字节数变成前缀和，得到 len(sizes)+1 个块内偏移。
func offsetsOf(sizes []int) []int {
	offs := make([]int, len(sizes)+1)
	for i, n := range sizes {
		offs[i+1] = offs[i] + n
	}
	return offs
}
