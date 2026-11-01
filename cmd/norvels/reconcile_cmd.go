package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"norvels/internal/store"
)

// cmdReconcileContent 对账主库与正文库（0018 起正文独立成文件）。
//
// 两库之间没有外键也没有跨库事务，写入靠「顺序 + 补偿」；这套机制唯一会留下痕迹的地方
// 就是这两类漂移，**对账是运维的最后一道闸**：
//
//	norvels reconcile-content [--db ...] [--fix]
//	  （无 flag）  只读体检：报孤儿块与悬空引用条数，有问题时退出码 1
//	  --fix       删掉孤儿块（book_id 不在主库的那些）；悬空引用只报不修
//
// 悬空引用（章节指着不存在的块）不自动修：缺的正文无法凭空造出来，
// 出路是从备份恢复或重新导入那本书，工具不该替人做这个决定。
func cmdReconcileContent(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("reconcile-content", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	fix := fs.Bool("fix", false, "删除孤儿块（book_id 不在主库的块）")
	fs.Parse(args)

	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	rep, err := st.ReconcileContent(ctx, *fix)
	if err != nil {
		fmt.Fprintf(os.Stdout, "FAIL 对账无法完成：%v\n", err)
		return err
	}
	printReconcile(rep, *fix)
	return reconcileResult(rep)
}

// printReconcile 打印一次对账的结果（运维的第一手证据）。
func printReconcile(rep store.ContentReconcileReport, fix bool) {
	if rep.MissingTable {
		fmt.Fprintln(os.Stdout, "正文库没有 chapter_blocks：库还没迁到 0018，或正文库文件丢失")
		return
	}
	fmt.Fprintf(os.Stdout, "孤儿块 %d 个（%d 本书）· 悬空引用 %d 章\n",
		rep.OrphanBlocks, rep.OrphanBooks, rep.DanglingRefs)
	if fix {
		fmt.Fprintf(os.Stdout, "已删除孤儿块 %d 个\n", rep.RemovedBlocks)
	}
	if rep.RemovedBlocks > 0 {
		fmt.Fprintln(os.Stdout, "提示：删除后的空闲页要跑一次 VACUUM 才会真正归还磁盘（见 docs/deploy.md §7.3）")
	}
}

// reconcileResult 把结果翻成退出码：有悬空引用即非零（那意味着有章读不出正文）。
func reconcileResult(rep store.ContentReconcileReport) error {
	if rep.DanglingRefs > 0 {
		return fmt.Errorf("有 %d 章指向不存在的正文块：从备份恢复正文库，或重新导入对应书籍", rep.DanglingRefs)
	}
	return nil
}
