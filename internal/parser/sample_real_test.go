package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSampleRealCorpus 抽样真实语料做整体体检。
//
// 用法（默认跳过，避免 CI 依赖外部数据）：
//
//	NORVELS_SAMPLE_DIR="/path/to/txt" NORVELS_SAMPLE_N=120 go test ./internal/parser/ -run TestSampleRealCorpus -v
//
// 输出：成功/失败数、编码分布、章节数分布、与文件名区间的对照、作者抽取率、
// 0/1 章清单、MultiBook 数量、超大单章清单、最慢文件。
func TestSampleRealCorpus(t *testing.T) {
	dir := os.Getenv("NORVELS_SAMPLE_DIR")
	if dir == "" {
		t.Skip("未设置 NORVELS_SAMPLE_DIR，跳过真实语料抽样")
	}
	limit := envInt("NORVELS_SAMPLE_N", 120)
	stride := envInt("NORVELS_SAMPLE_STRIDE", 1)

	files, err := collectTxt(dir)
	if err != nil {
		t.Fatalf("扫描目录失败: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("目录中没有 txt: %s", dir)
	}
	sampled := make([]string, 0, limit)
	for i := 0; i < len(files) && len(sampled) < limit; i += stride {
		sampled = append(sampled, files[i])
	}

	p := New(DefaultOptions())
	encCount := make(map[string]int)
	type stat struct {
		name      string
		enc       string
		chapters  int
		volumes   int
		words     int
		bytes     int64
		elapsed   time.Duration
		title     string
		author    string
		multi     bool
		maxChap   int
		chapterNo []int
		warnings  []string
	}
	var stats []stat
	var failures, zeros, ones, bigChaps, badTitles []string
	authorOK, authorMissing := 0, 0
	startAll := time.Now()

	for _, path := range sampled {
		b, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, filepath.Base(path)+"  读文件失败: "+err.Error())
			continue
		}
		st := time.Now()
		book, err := p.ParseBytes(filepath.Base(path), b)
		el := time.Since(st)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s  %v", filepath.Base(path), err))
			continue
		}
		encCount[book.Encoding]++
		maxChap := 0
		for _, c := range book.Chapters {
			if len(c.Content) > maxChap {
				maxChap = len(c.Content)
			}
		}
		if book.Author == "佚名" || book.Author == "" {
			authorMissing++
		} else {
			authorOK++
		}
		switch book.ChapterCount() {
		case 0:
			zeros = append(zeros, filepath.Base(path))
		case 1:
			ones = append(ones, fmt.Sprintf("%s  字=%d 编码=%s", filepath.Base(path), book.WordCount, book.Encoding))
		}
		if maxChap > 100000 {
			bigChaps = append(bigChaps, fmt.Sprintf("%s  最大单章=%d 字节 章数=%d", filepath.Base(path), maxChap, book.ChapterCount()))
		}
		if t2 := suspiciousTitle(book.Title); t2 != "" {
			badTitles = append(badTitles, fmt.Sprintf("%s => %q（%s）", filepath.Base(path), book.Title, t2))
		}
		stats = append(stats, stat{
			name: filepath.Base(path), enc: book.Encoding, chapters: book.ChapterCount(),
			volumes: len(book.Volumes), words: book.WordCount, bytes: book.FileSize,
			elapsed: el, title: book.Title, author: book.Author, multi: book.MultiBook,
			maxChap: maxChap, warnings: book.Warnings,
		})
	}

	buckets := map[string]int{"0": 0, "1": 0, "2-9": 0, "10-49": 0, "50-199": 0, "200-999": 0, "1000+": 0}
	fallback := 0
	multi := 0
	for _, s := range stats {
		switch c := s.chapters; {
		case c == 0:
			buckets["0"]++
		case c == 1:
			buckets["1"]++
		case c < 10:
			buckets["2-9"]++
		case c < 50:
			buckets["10-49"]++
		case c < 200:
			buckets["50-199"]++
		case c < 1000:
			buckets["200-999"]++
		default:
			buckets["1000+"]++
		}
		if s.multi {
			multi++
		}
		for _, w := range s.warnings {
			if strings.Contains(w, "兜底切分") || strings.Contains(w, "未发现章节标记") {
				fallback++
				break
			}
		}
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].elapsed > stats[j].elapsed })
	encKeys := make([]string, 0, len(encCount))
	for k := range encCount {
		encKeys = append(encKeys, k)
	}
	sort.Strings(encKeys)
	encParts := make([]string, 0, len(encKeys))
	for _, k := range encKeys {
		encParts = append(encParts, fmt.Sprintf("%s=%d", k, encCount[k]))
	}

	t.Logf("==== 真实语料抽样报告 ====")
	t.Logf("目录: %s", dir)
	t.Logf("样本: %d / %d 个文件，总耗时 %s", len(sampled), len(files), time.Since(startAll).Round(time.Millisecond))
	t.Logf("成功 %d，失败 %d", len(stats), len(failures))
	t.Logf("编码分布: %s", strings.Join(encParts, " "))
	t.Logf("章节数分布: 0=%d 1=%d 2-9=%d 10-49=%d 50-199=%d 200-999=%d 1000+=%d（兜底切分 %d）",
		buckets["0"], buckets["1"], buckets["2-9"], buckets["10-49"], buckets["50-199"], buckets["200-999"], buckets["1000+"], fallback)
	t.Logf("作者抽取: 有作者 %d，佚名 %d（抽取率 %.1f%%）",
		authorOK, authorMissing, 100*float64(authorOK)/float64(max(1, authorOK+authorMissing)))
	t.Logf("MultiBook 标记: %d", multi)

	// 与文件名里的区间对照
	matched, mismatched := 0, 0
	var mismatchLines []string
	for _, s := range stats {
		lo, hi, ok := parseNameRange(s.name)
		if !ok || hi <= 0 {
			continue
		}
		switch {
		case s.chapters >= lo && s.chapters <= hi*2:
			matched++
		default:
			mismatched++
			mismatchLines = append(mismatchLines, fmt.Sprintf("%s  文件名区间 %d-%d 实际章=%d", truncRunes(s.name, 46), lo, hi, s.chapters))
		}
	}
	if matched+mismatched > 0 {
		t.Logf("章节数与文件名区间对照: 落在区间内或 2 倍以内 %d，偏离 %d", matched, mismatched)
		for _, l := range mismatchLines {
			t.Logf("  偏离: %s", l)
		}
	}

	t.Logf("最慢 5 个文件:")
	for i := 0; i < len(stats) && i < 5; i++ {
		s := stats[i]
		t.Logf("  %-44s %7.0fms %6.1fMB 章=%-6d 编码=%s", truncRunes(s.name, 42),
			float64(s.elapsed.Microseconds())/1000.0, float64(s.bytes)/1048576.0, s.chapters, s.enc)
	}
	if len(failures) > 0 {
		t.Logf("失败清单（%d）:", len(failures))
		for _, f := range failures {
			t.Logf("  %s", f)
		}
	}
	if len(zeros) > 0 {
		t.Logf("0 章清单（%d）:", len(zeros))
		for _, f := range zeros {
			t.Logf("  %s", f)
		}
	}
	if len(ones) > 0 {
		t.Logf("1 章清单（%d）:", len(ones))
		for _, f := range ones {
			t.Logf("  %s", f)
		}
	}
	if len(bigChaps) > 0 {
		t.Logf("单章 >100KB 清单（%d）:", len(bigChaps))
		for _, f := range bigChaps {
			t.Logf("  %s", f)
		}
	}
	if len(badTitles) > 0 {
		t.Logf("可疑书名（%d）:", len(badTitles))
		for _, f := range badTitles {
			t.Logf("  %s", f)
		}
	}
	if len(stats)*10 < len(sampled)*9 {
		t.Errorf("解析成功率过低: %d/%d", len(stats), len(sampled))
	}
}

// suspiciousTitle 检查书名是否明显有问题（残留括号/过长/为空）。
func suspiciousTitle(t string) string {
	switch {
	case t == "":
		return "空书名"
	case runeLen(t) > 40:
		return "过长"
	case strings.ContainsAny(t, "【】[]") && runeLen(t) > 0:
		return "残留括号"
	case regexp.MustCompile("^(?:第?\\d+[章节回])$").MatchString(t):
		return "疑似章节标题"
	}
	return ""
}

// parseNameRange 从文件名里解析「(1-1990)」「0001-6177」「更新至925章」这类章节区间。
func parseNameRange(name string) (lo, hi int, ok bool) {
	re := regexp.MustCompile("(\\d{1,5})\\s*[-~－至到_]\\s*(\\d{1,5})")
	if m := re.FindStringSubmatch(name); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > 0 && b > a {
			return a, b, true
		}
	}
	re2 := regexp.MustCompile("(?:更新至|至)(\\d{1,5})\\s*章")
	if m := re2.FindStringSubmatch(name); m != nil {
		a, _ := strconv.Atoi(m[1])
		if a > 0 {
			return 1, a, true
		}
	}
	return 0, 0, false
}

func collectTxt(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".txt") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func truncRunes(s string, n int) string {
	return truncateRunes(s, n)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
