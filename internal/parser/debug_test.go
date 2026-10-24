package parser

import (
	"fmt"
	"os"
	"testing"
)

// TestDebugOneFile 打印单个文件的解析细节（排障用，默认跳过）。
//
//	NORVELS_DEBUG_FILE=/path/to/foo.txt NORVELS_DEBUG_N=20 go test ./internal/parser/ -run TestDebugOneFile -v
func TestDebugOneFile(t *testing.T) {
	path := os.Getenv("NORVELS_DEBUG_FILE")
	if path == "" {
		t.Skip("未设置 NORVELS_DEBUG_FILE")
	}
	limit := envInt("NORVELS_DEBUG_N", 20)
	p := New(DefaultOptions())
	book, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	t.Logf("书名=%q 作者=%q 编码=%s 章=%d 卷=%d 字=%d 多书=%v 警告=%v",
		book.Title, book.Author, book.Encoding, book.ChapterCount(), len(book.Volumes),
		book.WordCount, book.MultiBook, book.Warnings)
	for i, c := range book.Chapters {
		if i >= limit {
			break
		}
		t.Logf("  #%-3d vol=%-2d vol=%-2d %-30q bytes=%-7d 首行=%s", c.Idx, c.VolumeIdx, c.VolumeIdx,
			truncRunes(c.Title, 28), len(c.Content), firstLine(c.Content))
	}
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return truncRunes(s[:i], 40)
		}
	}
	return truncRunes(s, 40)
}

var _ = fmt.Sprintf
