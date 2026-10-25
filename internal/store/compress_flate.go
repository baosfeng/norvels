package store

import (
	"bytes"
	"compress/flate"
	"fmt"
)

// 历史格式支持：块 BLOB 的 0x01 标记 = flate-6 raw deflate。
//
// 只有「正文压缩骨架期」写入过的库才会有这种块（本项目从未发布过该格式，
// 但格式位一旦写进磁盘就该永远读得回来，否则将来改算法就是一次静默数据丢失）。
// 写入侧现已统一用 0x02 zstd，这里只保留读取能力。

// inflateBlock 解开一段 flate raw deflate 载荷。
func inflateBlock(payload []byte) ([]byte, error) {
	zr := flate.NewReader(bytes.NewReader(payload))
	defer zr.Close() //nolint:errcheck // 只读句柄，Close 不产生业务错误
	var out bytes.Buffer
	out.Grow(len(payload) * 2)
	if _, err := out.ReadFrom(zr); err != nil {
		return nil, fmt.Errorf("%w: flate: %v", ErrCorruptContent, err)
	}
	return out.Bytes(), nil
}
