package main

import (
	"bytes"
	"compress/flate"
	"testing"
)

// flateBlob 造一个 0x01 标记的库内块（历史格式，只读路径仍需认得）。
func flateBlob(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		t.Fatalf("建 flate writer 失败: %v", err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("flate 写入失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate 关闭失败: %v", err)
	}
	return append([]byte{storedFlate}, buf.Bytes()...)
}

// TestDecompressStoredMarkers 锁住只读路径认得库内全部四种块标记。
//
// 迁移 0017 之后库内全是 0x03（zstd 压码本流）：少认一种，工具在真库上会整体读不动，
// 报「未知的库内算法标记」而不是把正文交出来。
func TestDecompressStoredMarkers(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	raw := []byte("第一章 正文往返测试，含标点与 ASCII content。")
	plain, err := packOne(c, raw, false)
	if err != nil {
		t.Fatalf("打包纯 zstd 块失败: %v", err)
	}
	coded, err := packOne(c, raw, true)
	if err != nil {
		t.Fatalf("打包码本块失败: %v", err)
	}
	if plain[0] != storedZstd || coded[0] != storedCodebook {
		t.Fatalf("打包标记错误：plain=0x%02x coded=0x%02x", plain[0], coded[0])
	}

	cases := []struct {
		name   string
		packed []byte
	}{
		{"0x00 原文", append([]byte{storedRaw}, raw...)},
		{"0x01 flate", flateBlob(t, raw)},
		{"0x02 zstd", plain},
		{"0x03 码本", coded},
	}
	for _, tc := range cases {
		got, derr := decompressStored(c, tc.packed)
		if derr != nil {
			t.Fatalf("%s: 解不出来: %v", tc.name, derr)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("%s: 还原不一致: %q", tc.name, got)
		}
	}
}

// TestDecompressStoredEmpty 空块返回空，不是错误（库里「整块正文皆空」不建块行）。
func TestDecompressStoredEmpty(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()
	got, err := decompressStored(c, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("空块应返回空且无错，得到 %q / %v", got, err)
	}
}

// TestDecompressStoredRejectsCorrupt 未知标记与损坏载荷必须报错，不能静默返回空正文。
func TestDecompressStoredRejectsCorrupt(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	bad := []struct {
		name   string
		packed []byte
	}{
		{"未知标记 0x7F", []byte{0x7F, 0x00}},
		{"zstd 载荷损坏", []byte{storedZstd, 0xFF, 0x00, 0x01}},
		{"码本载荷损坏", []byte{storedCodebook, 0xFF, 0x00, 0x01}},
		{"flate 载荷损坏", []byte{storedFlate, 0xFF, 0x00, 0x01}},
	}
	for _, tc := range bad {
		if _, derr := decompressStored(c, tc.packed); derr == nil {
			t.Fatalf("%s: 必须报错，否则坏块会被当成空正文", tc.name)
		}
	}
}

// TestUnpackOneEmptyAndCode 校验打包/解包的边界：空块与码本流。
func TestUnpackOneEmptyAndCode(t *testing.T) {
	c, err := newCompressor()
	if err != nil {
		t.Fatalf("建压缩器失败: %v", err)
	}
	defer c.Close()

	if got, uerr := unpackOne(c, nil); uerr != nil || len(got) != 0 {
		t.Fatalf("空 BLOB 应返回空且无错，得到 %q / %v", got, uerr)
	}
	raw := []byte("码本流往返")
	blob, perr := packOne(c, raw, true)
	if perr != nil {
		t.Fatalf("打包失败: %v", perr)
	}
	got, uerr := unpackOne(c, blob)
	if uerr != nil || !bytes.Equal(got, raw) {
		t.Fatalf("码本流往返不一致: %q / %v", got, uerr)
	}
	if _, cerr := unpackOne(c, []byte{storedZstd, 0xFF}); cerr == nil {
		t.Fatal("损坏 BLOB 必须报错")
	}
}
