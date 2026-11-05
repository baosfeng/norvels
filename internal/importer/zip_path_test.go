package importer

import (
	"path/filepath"
	"testing"
)

// TestUniqueEntryPathDeduplicates 不同包内目录下的同名条目（如同名「正文.txt」）
// 解包到同一目录时加序号后缀，后一个不会覆盖前一个。
func TestUniqueEntryPathDeduplicates(t *testing.T) {
	dst := t.TempDir()
	seen := map[string]int{}
	first, err := uniqueEntryPath(dst, "第一卷/正文.txt", seen)
	if err != nil {
		t.Fatalf("第一个条目: %v", err)
	}
	second, err := uniqueEntryPath(dst, "第二卷/正文.txt", seen)
	if err != nil {
		t.Fatalf("第二个条目: %v", err)
	}
	if first == second {
		t.Fatalf("重名条目落到同一路径 %q，后者会覆盖前者", first)
	}
	if want := filepath.Join(dst, "正文-1.txt"); second != want {
		t.Fatalf("第二个条目 = %q，want %q", second, want)
	}
}
