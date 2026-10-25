package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"norvels/internal/store"
)

// cmdCheckContent 校验章节正文可读。
//
// 这是给运维在**迁移之后**自查用的：0006 删了分词与 FTS，0007 把正文搬进了压缩块，
// 两次都动了「用户唯一在读的东西」。
//
// 用法（--all 与 --sample 互斥）：
//
//	norvels check-content [--db ...] [--all | --sample N]
//	  --all       全量校验所有章节（慢，迁移后建议跑一次）
//	  --sample N  随机抽 N 章（默认 50）
func cmdCheckContent(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("check-content", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	all := fs.Bool("all", false, "全量校验所有章节（慢，迁移后建议跑一次）")
	sample := fs.Int("sample", defaultCheckSample, "随机抽 N 章")
	fs.Parse(args)

	opt, err := contentCheckOptions(*all, *sample, fs)
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	rep, err := st.CheckContent(ctx, opt)
	if err != nil {
		fmt.Fprintf(os.Stdout, "FAIL 正文校验无法完成：%v\n", err)
		return err
	}
	if rep.TotalChapters == 0 {
		fmt.Fprintln(os.Stdout, "OK（空库，无章节可校验）")
		return nil
	}
	printContentCheck(rep, opt.All)
	return contentCheckResult(rep)
}

// defaultCheckSample 与 store 层默认值保持一致（此处只是 flag 的展示默认值）。
const defaultCheckSample = 50

// contentCheckOptions 解析 --all / --sample 的互斥语义。
//
// 互斥必须显式报错而不是「--all 悄悄赢」：运维敲了 --all --sample 10 时，
// 他到底想要全量还是抽样是有歧义的，猜错会让人误以为全量跑过了。
func contentCheckOptions(all bool, sample int, fs *flag.FlagSet) (store.ContentCheckOptions, error) {
	sampleSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "sample" {
			sampleSet = true
		}
	})
	if all && sampleSet {
		return store.ContentCheckOptions{}, fmt.Errorf("--all 与 --sample 互斥，二选一")
	}
	return store.ContentCheckOptions{All: all, Sample: sample}, nil
}

// printContentCheck 打印自查摘要。
//
// 输出刻意包含「触及多少块」：若校验 50 章却只触及 1 块，说明块切得过大，
// 该书的翻页延迟会受影响；正常情况下每块约 1 MB，触及的块数应与校验数同量级。
func printContentCheck(rep store.ContentCheckReport, all bool) {
	mode := "抽查"
	if all {
		mode = "全量校验"
	}
	fmt.Fprintf(os.Stdout, "%s %d/%d 章，触及 %d 个压缩块\n",
		mode, rep.Sampled, rep.TotalChapters, rep.Blocks)
	okChapters := rep.Sampled - rep.FailedChapters
	fmt.Fprintf(os.Stdout, "成功读出 %d 章，正文总字节 %d，单章最小 %d、最大 %d（平均 %d）\n",
		okChapters, rep.TotalBytes, rep.MinBytes, rep.MaxBytes, avgBytes(rep.TotalBytes, okChapters))
	if rep.FailedChapters > 0 {
		fmt.Fprintf(os.Stdout, "读取失败 %d 章（涉及 %d 个损坏块）\n",
			rep.FailedChapters, len(rep.Corrupted))
	}
	if rep.EmptyChapters > 0 {
		fmt.Fprintf(os.Stdout, "其中空章 %d 个（空章合法，仅供参考）\n", rep.EmptyChapters)
	}
}

// contentCheckResult 输出结论并决定是否返回错误。
//
// ## 判定口径：只认存储完整性
//
// FAIL 的唯一依据是**块损坏**（解压失败 / 区间越界 / 块缺失）——只有那些才意味着
// 「读回来的字节 != 当初写进去的字节」。
//
// 正文不是合法 UTF-8 **不算失败**：它是源 txt 的编码问题（常见于某个文件主体 GBK、
// 夹了一段 UTF-8），迁移逐字节搬运时原样保留了下来。把它报成 FAIL 会有两个坏后果：
// 用户以为迁移把数据搞坏了，排障方向也被引到存储层。所以这里只打一行提示，退出码 0。
//
// 有坏块时**列出全部**而不是只报第一个：几万本书的库里可能同时有多处磁盘损坏，
// 一次列全才能一轮修完。
func contentCheckResult(rep store.ContentCheckReport) error {
	if len(rep.Corrupted) > 0 {
		fmt.Fprintf(os.Stdout, "FAIL 发现 %d 个损坏的压缩块：\n", len(rep.Corrupted))
		for _, line := range rep.Errors() {
			fmt.Fprintf(os.Stdout, "  - %s\n", line)
		}
		fmt.Fprintln(os.Stdout, "这些块覆盖的章节都无法读出；建议从升级前的备份恢复，或用 --sample/--all 对照定位。")
		return fmt.Errorf("check-content: %d 个压缩块损坏", len(rep.Corrupted))
	}
	if rep.BadUTF8 > 0 {
		printBadUTF8Notice(rep)
	}
	fmt.Fprintln(os.Stdout, "OK")
	return nil
}

// printBadUTF8Notice 输出「源数据编码问题」的提示。
//
// 措辞刻意区分「存储完好」与「内容有脏数据」，并明确这不是迁移造成的，
// 避免运维照着错误的线索去查压缩块或备份。
func printBadUTF8Notice(rep store.ContentCheckReport) {
	fmt.Fprintf(os.Stdout,
		"提示：%d 章正文含非法 UTF-8（源文件编码问题，非迁移损坏，存储完整性不受影响）\n",
		rep.BadUTF8)
	if len(rep.BadUTF8ChapterIDs) > 0 {
		fmt.Fprintf(os.Stdout, "  涉及章节 id（最多列 %d 个）：%v\n",
			len(rep.BadUTF8ChapterIDs), rep.BadUTF8ChapterIDs)
	}
	fmt.Fprintln(os.Stdout, "  这些章节阅读时可能显示乱码；如需定位源文件，可用 --sample N 缩小范围后按 id 反查。")
}

// avgBytes 返回平均值（除数为 0 时返回 0）。
func avgBytes(total int64, n int) int64 {
	if n <= 0 {
		return 0
	}
	return total / int64(n)
}
