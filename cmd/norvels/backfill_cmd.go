package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"norvels/internal/store"
)

// cmdBackfillFingerprints 给存量书补写章级内容指纹（0019 只建表，不回填历史数据）。
//
// 为什么必须有这个命令：导入判重只认指纹，而 0019 上线前入的书一本都没有指纹——
// 不回填的话新导入的书永远比不到老书，去重等于没开。升级到 0019 之后跑一次即可。
//
//	norvels backfill-fingerprints [--db ...] [--batch N]
//
// 幂等：已有指纹的书整本跳过（正文没变过，重写只是白烧 CPU）；中断后重跑从断点继续；
// 单本书的指纹在一个正文库事务里写完，不会留下「半本书有指纹」。--batch 只决定
// 进度打印粒度，不改变事务粒度。
func cmdBackfillFingerprints(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("backfill-fingerprints", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	batch := fs.Int("batch", 0, "每批处理的书数（0 = 默认 200，只影响进度粒度）")
	fs.Parse(args)

	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck // CLI 退出路径

	fmt.Fprintf(os.Stdout, "库: %s\n", *dbPath)
	rep, err := st.BackfillFingerprints(ctx, store.FingerprintBackfillOptions{
		Batch:    *batch,
		Progress: printBackfillProgress,
	})
	printBackfillReport(rep)
	if err != nil {
		fmt.Fprintf(os.Stdout, "FAIL 回填中断：%v\n已写入的指纹保留，重新执行会从断点继续\n", err)
		return err
	}
	return nil
}

// printBackfillProgress 打一行进度：生产上回填要几十分钟，运维靠它判断任务还在动。
func printBackfillProgress(done, total int, elapsed time.Duration) {
	fmt.Fprintf(os.Stdout, "进度 %d/%d 本 · 用时 %s\n", done, total, elapsed.Round(time.Second))
}

// printBackfillReport 打印回填汇总：运维据此判断「要不要再跑」与「有没有坏块」。
func printBackfillReport(rep store.FingerprintBackfillReport) {
	if rep.Filled == 0 {
		fmt.Fprintf(os.Stdout, "无需回填：扫描 %d 本，全部已有指纹（跳过 %d）\n", rep.Books, rep.Skipped)
	} else {
		fmt.Fprintf(os.Stdout, "回填完成：扫描 %d 本（回填 %d / 已有指纹跳过 %d / 无正文 %d）\n",
			rep.Books, rep.Filled, rep.Skipped, rep.NoContent)
		fmt.Fprintf(os.Stdout, "写入指纹 %d 行 · 坏章节 %d 章 · 用时 %s\n",
			rep.Fingerprints, rep.BadChapters, rep.Elapsed.Round(time.Millisecond))
	}
	if rep.BadChapters > 0 {
		fmt.Fprintln(os.Stdout, "有坏章节：跑 norvels check-content --all 定位（坏块不影响其它书的回填）")
	}
}
