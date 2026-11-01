package main

import (
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/store"
)

// TestPrintReconcile 覆盖对账输出：三个数（孤儿块/孤儿书/悬空引用）与 --fix 的删除条数
// 是运维判断「要不要从备份恢复」的全部依据，缺一个都读不懂现场。
func TestPrintReconcile(t *testing.T) {
	rep := store.ContentReconcileReport{OrphanBlocks: 3, OrphanBooks: 1, DanglingRefs: 2}
	out := captureStdout(t, func() { printReconcile(rep, false) })
	for _, want := range []string{"孤儿块 3 个", "1 本书", "悬空引用 2 章"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
	if strings.Contains(out, "已删除") {
		t.Errorf("没给 --fix 不该打印删除条数：\n%s", out)
	}
}

// TestPrintReconcileFixMode 给了 --fix 要回报实际删除的块数。
func TestPrintReconcileFixMode(t *testing.T) {
	out := captureStdout(t, func() {
		printReconcile(store.ContentReconcileReport{OrphanBlocks: 5, OrphanBooks: 2, RemovedBlocks: 5}, true)
	})
	for _, want := range []string{"已删除孤儿块 5 个", "VACUUM"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
}

// TestPrintReconcileMissingTable 正文库没有块表时要说清楚「还没迁到 0018 或文件丢了」，
// 而不是打一串 0 让人以为库是干净的。
func TestPrintReconcileMissingTable(t *testing.T) {
	out := captureStdout(t, func() { printReconcile(store.ContentReconcileReport{MissingTable: true}, false) })
	if !strings.Contains(out, "0018") {
		t.Errorf("缺表提示不明确：\n%s", out)
	}
}

// TestReconcileResult 只有悬空引用让退出码非零：孤儿块只是体积问题。
func TestReconcileResult(t *testing.T) {
	if err := reconcileResult(store.ContentReconcileReport{OrphanBlocks: 9}); err != nil {
		t.Errorf("只有孤儿块不该报错：%v", err)
	}
	if err := reconcileResult(store.ContentReconcileReport{DanglingRefs: 1}); err == nil {
		t.Error("有悬空引用必须让退出码非零")
	}
}

// TestCmdReconcileContent 端到端跑一遍子命令：新建库 → 迁移 → 对账干净 → --fix 也干净。
//
// 覆盖的是「运维真正会敲的那条命令」：flag 解析 + 打开双库 + 打印 + 退出码。
func TestCmdReconcileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.db")
	out := captureStdout(t, func() {
		if err := cmdReconcileContent([]string{"--db", path}); err != nil {
			t.Fatalf("cmdReconcileContent: %v", err)
		}
	})
	if !strings.Contains(out, "孤儿块 0 个") || !strings.Contains(out, "悬空引用 0 章") {
		t.Errorf("空库对账输出异常：\n%s", out)
	}
	if err := cmdReconcileContent([]string{"--db", path, "--fix"}); err != nil {
		t.Fatalf("cmdReconcileContent --fix: %v", err)
	}
}
