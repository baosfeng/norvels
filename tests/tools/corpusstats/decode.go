package main

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// 块 BLOB 的算法标记：0x00 raw / 0x01 flate / 0x02 zstd。
// 与 internal/store/compress.go 的常量一一对应（格式变更必须同步，否则统计口径失效）。
const (
	algoRaw   byte = 0x00
	algoFlate byte = 0x01
	algoZstd  byte = 0x02
)

// errCorrupt 表示块字节无法按标记解开（未知标记或压缩流损坏）。
var errCorrupt = errors.New("块内容无法按算法标记解开")

// zstdDecoders 复用解码器：每块新建会反复分配窗口缓冲，全库 8 万块代价明显。
var zstdDecoders = sync.Pool{New: func() any {
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		panic("corpusstats: 初始化 zstd 解码器: " + err.Error())
	}
	return dec
}}

// decompress 把一块「标记 + 载荷」还原成原始 UTF-8 字节。
func decompress(packed []byte) ([]byte, error) {
	if len(packed) == 0 {
		return nil, nil
	}
	switch packed[0] {
	case algoRaw:
		return packed[1:], nil
	case algoFlate:
		return inflate(packed[1:])
	case algoZstd:
		return unzstd(packed[1:])
	default:
		return nil, fmt.Errorf("%w: 未知算法标记 0x%02x", errCorrupt, packed[0])
	}
}

// inflate 解 raw deflate（flate 写端默认无 zlib 头）。
func inflate(payload []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(payload))
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: flate: %v", errCorrupt, err)
	}
	return out, nil
}

// unzstd 用池化解码器解一块 zstd。
func unzstd(payload []byte) ([]byte, error) {
	dec, ok := zstdDecoders.Get().(*zstd.Decoder)
	if !ok {
		return nil, errors.New("corpusstats: zstd 解码器池类型不符")
	}
	defer zstdDecoders.Put(dec)
	out, err := dec.DecodeAll(payload, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: zstd: %v", errCorrupt, err)
	}
	return out, nil
}
