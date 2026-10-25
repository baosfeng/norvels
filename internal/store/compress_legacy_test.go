package store

import (
	"bytes"
	"compress/flate"
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
