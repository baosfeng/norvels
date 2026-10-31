package store

import (
	"bytes"
	"compress/flate"

	"github.com/klauspost/compress/zstd"
)

// compressLegacyFlate 是测试专用的 flate 块构造器：模拟「骨架期写入的 0x01 块」，
// 用来验证读取侧仍兼容历史格式。生产代码不会再产出这种块。
func compressLegacyFlate(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(contentAlgoFlate)
	zw, err := flate.NewWriter(&buf, 6)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// compressLegacyZstd 是测试专用的 0x02 块构造器：模拟「码本上线之前写入的 zstd 块」，
// 用来验证读取侧仍能读出老库里的块。写入侧已不再产出这个标记。
func compressLegacyZstd(raw []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, err
	}
	defer enc.Close() //nolint:errcheck // 只写句柄，关闭不产生业务错误
	return enc.EncodeAll(raw, []byte{contentAlgoZstd}), nil
}
