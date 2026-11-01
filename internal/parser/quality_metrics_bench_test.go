package parser

import (
	"strconv"
	"strings"
	"sync"
	"testing"
)

// benchCorpusBytes 是基准语料的目标体积：40 MB 量级，对应生产里最大的单本 txt。
const benchCorpusBytes = 40 << 20

var (
	corpusOnce sync.Once
	corpusText string
)

// benchCorpus 返回约 40 MB 的合成正文（虚构书名与正文，段落互不相同）。
func benchCorpus() string {
	corpusOnce.Do(func() {
		var b strings.Builder
		b.Grow(benchCorpusBytes + 4096)
		for i := 1; b.Len() < benchCorpusBytes; i++ {
			b.WriteString("第" + strconv.Itoa(i) + "章 合成标题\n")
			for j := 0; j < 20; j++ {
				b.WriteString("这是用于基准测试的合成正文段落，内容本身没有意义，只保证长度与标点形态接近中文小说，编号")
				b.WriteString(strconv.Itoa(i) + "-" + strconv.Itoa(j) + "。\n")
			}
		}
		corpusText = b.String()
	})
	return corpusText
}

// newBenchDoc 用真实管线（过滤 → 切章 → 后处理，跳过解码）构造一份 Doc，
// 让「只测指标」的基准拿到与生产同形的章节和清洗统计。
func newBenchDoc(b *testing.B, text string) *Doc {
	b.Helper()
	p := New(DefaultOptions())
	d := &Doc{Name: "合成书名.txt", Raw: []byte(text), Text: text, opts: p.Opts}
	runFilters(d, nil, nil)
	d.titleCandidates()
	runMetas(d, nil)
	runSplitter(d, "", nil)
	postProcessDoc(d)
	return d
}

// BenchmarkQualityMetrics40MB 只测质量指标的计算开销（正文单次遍历）。
func BenchmarkQualityMetrics40MB(b *testing.B) {
	text := benchCorpus()
	d := newBenchDoc(b, text)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeQuality(d)
	}
}

// BenchmarkParse40MB 测含质量指标的整条解析链路，用来算指标占比。
func BenchmarkParse40MB(b *testing.B) {
	raw := []byte(benchCorpus())
	p := New(DefaultOptions())
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.ParseBytes("合成书名.txt", raw); err != nil {
			b.Fatal(err)
		}
	}
}
