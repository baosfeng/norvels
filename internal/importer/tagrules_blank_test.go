package importer

import (
	"os"
	"path/filepath"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// TestMatchTagsBlankKeyword 空白关键词被跳过，其余关键词照常命中。
func TestMatchTagsBlankKeyword(t *testing.T) {
	rules := []store.TagRule{{Tag: "空词", Keywords: []string{"   ", "书名之书"}}}
	if got := matchTags(rules, &model.ParsedBook{Title: "书名之书"}); len(got) != 1 {
		t.Errorf("matchTags = %v，期望 1 个标签", got)
	}
}

// TestSourcesToCandidatesUnsupportedFile 源存在但扩展名不支持时记 failed。
func TestSourcesToCandidatesUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "说明.pdf")
	if err := os.WriteFile(pdf, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 pdf: %v", err)
	}
	cands, issues := sourcesToCandidates([]string{pdf}, dir)
	if len(cands) != 0 || len(issues) != 1 {
		t.Fatalf("cands/issues = %d/%d，期望 0/1（不支持扩展名）", len(cands), len(issues))
	}
}
