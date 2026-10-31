package textcode

import (
	"testing"
	"unicode/utf8"
)

// benchText 造一段「像正文」的中文：按全库字频的实际分布抽样——约 63% 的字落在 1 字节层、
// 37% 落在 2 字节层、0.007% 落在表外（3 字节兜底）。这样基准测的是真实负载，
// 而不是「整表轮转一遍」这种 1 字节层只占 2% 的假分布。
func benchText(n int) []byte {
	out := make([]byte, 0, 2*n)
	doubles := len(builtin.entries) - singlesCap
	for i := 0; i < n; i++ {
		idx := singlesCap + (i/100)%doubles // 2 字节层
		if i%100 < 63 {
			idx = (i / 100) % singlesCap // 1 字节层
		}
		out = utf8.AppendRune(out, builtin.entries[idx])
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
