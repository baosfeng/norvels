package importer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSingleFile 确认源路径可以直接是一个 txt 文件：只导这一本，
// 同目录的其他 txt 不受影响，recursive 开关对单文件也没有影响。
func TestRunSingleFile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "only.txt", sampleText("单本小说", "作者甲"))
	writeTxt(t, dir, "neighbor.txt", sampleText("隔壁小说", "作者乙"))
	path := filepath.Join(dir, "only.txt")

	for _, recursive := range []bool{true, false} {
		o := opts(path)
		o.Recursive = recursive
		o.Force = true // 第二次跑要跳过幂等检查，才能验证同样导入 1 本
		res, err := Run(ctx, st, o, nil)
		if err != nil {
			t.Fatalf("recursive=%v Run: %v", recursive, err)
		}
		if res.Job == nil || res.Job.Total != 1 {
			t.Fatalf("recursive=%v 任务总数 = %+v，期望只扫到 1 个文件", recursive, res.Job)
		}
		if len(res.Imported) != 1 || res.Failed != 0 {
			t.Fatalf("recursive=%v result = %+v，期望导入 1 本、0 失败", recursive, res)
		}
	}
}

// TestRunRejectsBadSingleFile 确认「不是 txt」与「路径不存在」都明确报错，
// 而不是掉进 "no .txt files found" 这种把用户引向错误方向的提示。
func TestRunRejectsBadSingleFile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "notes.md", "不是小说")

	t.Run("非 txt 文件", func(t *testing.T) {
		_, err := Run(ctx, st, opts(filepath.Join(dir, "notes.md")), nil)
		if err == nil || !strings.Contains(err.Error(), "不是 .txt 文件") {
			t.Fatalf("err = %v，期望明确提示不是 txt", err)
		}
	})
	t.Run("路径不存在", func(t *testing.T) {
		_, err := Run(ctx, st, opts(filepath.Join(dir, "missing.txt")), nil)
		if err == nil || !strings.Contains(err.Error(), "stat") {
			t.Fatalf("err = %v，期望 stat 错误", err)
		}
	})
}
