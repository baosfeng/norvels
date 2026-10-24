package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
)

// BenchmarkSaveBook 用合成中文正文测「解析后写入 + FTS 触发器同步」的吞吐，
// 用于外推全量 4.1GB 的导入耗时（不含 parser 解码/切分，那部分由 importer 实测覆盖）。
func BenchmarkSaveBook(b *testing.B) {
	ctx := context.Background()
	dir := b.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		b.Fatal(err)
	}

	const (
		chapterCount = 2000
		paragraphs   = 8
	)
	para := strings.Repeat("华夏国梅宁市，午后的秋日，微风拂过城楼。他站在城楼上望着远方，心中百感交集。", 3)
	chapters := make([]model.ParsedChapter, 0, chapterCount)
	totalBytes := 0
	for i := 0; i < chapterCount; i++ {
		var sb strings.Builder
		for j := 0; j < paragraphs; j++ {
			sb.WriteString(para)
		}
		content := sb.String()
		totalBytes += len(content)
		chapters = append(chapters, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: -1,
			Title:     fmt.Sprintf("第%d章 风起", i+1),
			Content:   content,
		})
	}

	b.SetBytes(int64(totalBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book := &model.ParsedBook{
			Title:      fmt.Sprintf("基准书%d", i),
			Author:     "基准作者",
			SourcePath: filepath.Join(dir, fmt.Sprintf("book%d.txt", i)),
			SourceName: fmt.Sprintf("book%d.txt", i),
			Encoding:   "utf-8",
			FileSize:   int64(totalBytes),
			SHA256:     fmt.Sprintf("%064d", i),
			WordCount:  chapterCount * 100,
			Chapters:   chapters,
		}
		if _, err := st.SaveBook(ctx, book, SaveOptions{Force: true}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = time.Now
}
