package parser

import (
	"strings"
	"testing"
)

// volumeOnlyFixture 构造一本「只有分卷标记、没有章节标记」的书（合成数据）。
func volumeOnlyFixture() string {
	var b strings.Builder
	b.WriteString("某书名\n作者：某作者\n\n")
	for i := 1; i <= 3; i++ {
		b.WriteString("第")
		b.WriteString([]string{"", "一", "二", "三"}[i])
		b.WriteString("部 卷标题")
		b.WriteString(string(rune('0' + i)))
		b.WriteString("\n")
		for j := 0; j < 40; j++ {
			b.WriteString("这是第")
			b.WriteString(string(rune('0' + i)))
			b.WriteString("部的正文段落，用来验证「卷即章」的切分是否正确。\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestPromoteVolumeOnly 覆盖「只有卷标记」这条主路径。
func TestPromoteVolumeOnly(t *testing.T) {
	opts := DefaultOptions()
	book, err := New(opts).ParseBytes("某书.txt", []byte(volumeOnlyFixture()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("章数 = %d，期望 3（每部一章）", len(book.Chapters))
	}
	for i, c := range book.Chapters {
		if !strings.Contains(c.Title, "第") {
			t.Errorf("第 %d 章标题 = %q，期望含卷标题", i+1, c.Title)
		}
		if c.WordCount == 0 {
			t.Errorf("第 %d 章字数为 0", i+1)
		}
	}
	if len(book.Volumes) != 0 {
		t.Errorf("卷即章后不应再保留卷分组，实际 %d 个", len(book.Volumes))
	}
	if !hasWarning(book.Warnings, "只有分卷标记") {
		t.Errorf("应记录降级告警，实际 %v", book.Warnings)
	}
}

// TestPromoteVolumeOnlySkipsWhenChaptersExist 覆盖「有章标记就不该走卷即章」。
func TestPromoteVolumeOnlySkipsWhenChaptersExist(t *testing.T) {
	var b strings.Builder
	b.WriteString("第一部 卷标题\n")
	for i := 1; i <= 5; i++ {
		b.WriteString("第")
		b.WriteString(string(rune('0' + i)))
		b.WriteString("章 标题\n正文\n")
	}
	book, err := New(DefaultOptions()).ParseBytes("某书.txt", []byte(b.String()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 有章标记时按章切，卷作为分组保留。
	if len(book.Chapters) != 5 {
		t.Fatalf("章数 = %d，期望 5", len(book.Chapters))
	}
	if len(book.Volumes) != 1 {
		t.Errorf("卷分组应保留，实际 %d 个", len(book.Volumes))
	}
}

// TestPromoteVolumeOnlyNeedsTwoVolumes 覆盖「只有 1 个卷标记不足以降级」。
func TestPromoteVolumeOnlyNeedsTwoVolumes(t *testing.T) {
	d := &Doc{Name: "某书.txt", Text: "第一部 标题\n正文", opts: DefaultOptions()}
	cands := []candidate{{h: heading{kind: headVolume, volTitle: "第一部"}}}
	if _, ok := promoteVolumeOnly(d, splitResult{}, cands); ok {
		t.Error("单个卷标记不应触发卷即章")
	}
}

// TestPromoteVolumeOnlyIgnoresStrayMarkers 覆盖「零星章标记」不阻止降级。
//
// 真实语料里正文常出现「见第三章」这类误判，一两个章标记不足以说明
// 书是按章写的。
func TestPromoteVolumeOnlyIgnoresStrayMarkers(t *testing.T) {
	d := &Doc{Name: "某书.txt", Text: strings.Repeat("正文段落。\n", 200), opts: DefaultOptions()}
	var cands []candidate
	cands = append(cands, candidate{h: heading{kind: headChapter, hasNum: true, num: 3}, start: 0, bodyStart: 10})
	for i := 1; i <= 3; i++ {
		cands = append(cands, candidate{
			h:     heading{kind: headVolume, volTitle: "第" + string(rune('0'+i)) + "部"},
			start: i * 100, bodyStart: i*100 + 10,
		})
	}
	if _, ok := promoteVolumeOnly(d, splitResult{preamble: "序言"}, cands); !ok {
		t.Error("零星 1 个章标记不应阻止卷即章")
	}
}

// hasWarning 判断告警列表里是否含指定片段。
func hasWarning(warns []string, sub string) bool {
	for _, w := range warns {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}
