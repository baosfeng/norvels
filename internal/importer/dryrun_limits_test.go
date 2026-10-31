package importer

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDryRunLimitsMatchExportedConstants 导出常量就是预检实际生效的上限：
// 样例条数与错误条数都以它们为准（改一处漏一处会在这里红）。
//
// files.total 的 truncated 上限已有 TestDryRunTruncated 覆盖，这里补另外两个。
func TestDryRunLimitsMatchExportedConstants(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	// 样例上限：写 DryRunMaxSamples+1 个可导入文件 → 报告只留 DryRunMaxSamples 条。
	dir := t.TempDir()
	for i := 0; i < DryRunMaxSamples+1; i++ {
		name := "s" + itoaTest(i) + ".txt"
		writeTxtFile(t, filepath.Join(dir, name), "书名：样例"+itoaTest(i)+"\n\n第1章 起\n正文。\n")
	}
	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(rep.Sample) != DryRunMaxSamples {
		t.Errorf("sample 条数 = %d，期望 %d", len(rep.Sample), DryRunMaxSamples)
	}

	// 错误上限：写 DryRunMaxErrors+1 个不支持的扩展名 → 错误清单只留 DryRunMaxErrors 条。
	dir2 := t.TempDir()
	for i := 0; i < DryRunMaxErrors+1; i++ {
		name := "e" + itoaTest(i) + ".pdf"
		writeTxtFile(t, filepath.Join(dir2, name), "x")
	}
	rep2, err := DryRun(ctx, st, Options{Dir: dir2, Recursive: true})
	if err != nil {
		t.Fatalf("DryRun(unsupported): %v", err)
	}
	if len(rep2.Errors) != DryRunMaxErrors {
		t.Errorf("errors 条数 = %d，期望 %d", len(rep2.Errors), DryRunMaxErrors)
	}
	if rep2.Files.Unsupported != DryRunMaxErrors+1 {
		t.Errorf("unsupported 计数 = %d，期望 %d（计数不受清单上限影响）",
			rep2.Files.Unsupported, DryRunMaxErrors+1)
	}
}
