package textcode

import (
	"testing"
	"unicode/utf8"
)

// benchText 造一段「像正文」的中文：按码位顺序取内置表里的字，前 192 个走 1 字节层，
// 其余走 2 字节层——真实语料训练出的码表也是这个分布（平均 1.36 字节/字）。
func benchText(n int) []byte {
	out := make([]byte, 0, 3*n)
	for i := 0; i < n; i++ {
		out = utf8.AppendRune(out, builtin.entries[i%len(builtin.entries)])
	}
	return out
}

// BenchmarkEncode 测编码吞吐：输入是 UTF-8 字节数，目标是 ≥30 MB/s。
func BenchmarkEncode(b *testing.B) {
	src := benchText(1 << 18)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Encode(nil, src); err != nil {
			b.Fatalf("Encode 失败: %v", err)
		}
	}
}

// BenchmarkDecode 测解码吞吐：输入是码本流字节数，目标是 ≥100 MB/s。
func BenchmarkDecode(b *testing.B) {
	enc, err := Encode(nil, benchText(1<<18))
	if err != nil {
		b.Fatalf("Encode 失败: %v", err)
	}
	b.SetBytes(int64(len(enc)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(nil, enc); err != nil {
			b.Fatalf("Decode 失败: %v", err)
		}
	}
}

// BenchmarkEncodeFallback 测长尾兜底路径：表外字符一律 3 字节，用来确认最坏情况不塌方。
func BenchmarkEncodeFallback(b *testing.B) {
	var src []byte
	for i := 0; i < 1<<17; i++ {
		src = utf8.AppendRune(src, rune(0x20000+i%100))
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Encode(nil, src); err != nil {
			b.Fatalf("Encode 失败: %v", err)
		}
	}
}
