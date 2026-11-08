package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"norvels/internal/enrich"
	"norvels/internal/store"
)

// splitReport 是切分命令的汇总。
type splitReport struct {
	Scanned  int
	Split    int
	Chapters int
	Failed   int
	// NoBoundary 是「字数够但找不到段落边界」的章数：这类章是整篇没有换行的
	// 抓取脏数据，按段落切无从下手，只能报出来交给人处理（不硬切字符）。
	NoBoundary int
	Elapsed    time.Duration
}

// runSplitLongChapters 把超长章节按段落边界切开（默认 dry-run）。
//
// 切分走 store.SplitChapter：它是「只改 block_off / block_len」的偏移改写，
// 不解压重压正文，几万字的章也是毫秒级；切点由 enrich.SplitCuts 这个纯函数算。
func runSplitLongChapters(ctx context.Context, st *store.Store, o splitOpts) error {
	started := time.Now()
	list, err := st.ListHugeChapters(ctx, o.minWords, o.limit)
	if err != nil {
		return err
	}
	fmt.Printf("超长章节：%d 章（阈值 %d 字，目标每章 %d 字）%s\n",
		len(list), o.minWords, o.targetWords, dryHint(o.apply))
	rep := splitReport{Scanned: len(list)}
	for _, h := range list {
		ch, err := st.GetChapter(ctx, h.BookID, h.ChapterID)
		content := ""
		if err == nil {
			content = ch.Content
		}
		if err != nil {
			rep.Failed++
			fmt.Fprintf(os.Stderr, "警告: 读取章节失败 book=%d chapter=%d: %v\n", h.BookID, h.ChapterID, err)
			continue
		}
		cuts := enrich.SplitCuts(content, o.targetWords, o.minWords)
		if len(cuts) == 0 {
			rep.NoBoundary++
			continue
		}
		rep.Split++
		rep.Chapters += len(cuts) + 1
		fmt.Printf("  书 %d · 章 %d《%s》 %d 字 → %d 段（切点 %d 个）\n",
			h.BookID, h.Index, h.Title, h.WordCount, len(cuts)+1, len(cuts))
		if !o.apply {
			continue
		}
		if _, err := st.SplitChapter(ctx, h.BookID, h.ChapterID, cuts); err != nil {
			rep.Failed++
			fmt.Fprintf(os.Stderr, "警告: 切分失败 book=%d chapter=%d: %v\n", h.BookID, h.ChapterID, err)
		}
	}
	rep.Elapsed = time.Since(started)
	fmt.Printf("切分完成：扫描 %d 章 / 可切 %d 章 → %d 章 / 无段落边界 %d / 失败 %d · 用时 %s%s\n",
		rep.Scanned, rep.Split, rep.Chapters, rep.NoBoundary, rep.Failed,
		rep.Elapsed.Round(time.Second), applyHint(o.apply))
	return nil
}

// printEnrichHeader 打印本次运行的参数与候选规模（先让人看见「要动多少」）。
func printEnrichHeader(o enrichOpts, want map[string]bool, cands []store.EnrichCandidate) {
	fields := make([]string, 0, 3)
	for _, f := range []string{store.EnrichFieldAuthor, store.EnrichFieldIntro, store.EnrichFieldCategory} {
		if want[f] {
			fields = append(fields, f)
		}
	}
	fmt.Printf("字段：%v · 候选 %d 本 · 并发 %d / QPS %.1f / 最多 %d 源 %s\n",
		fields, len(cands), o.workers, o.qps, o.maxSources, applyHint(o.apply))
}

// printPlan 打印 --plan 的清单（不联网）：只给规模与样例，便于运维先定 --limit。
func printPlan(cands []store.EnrichCandidate) {
	if len(cands) == 0 {
		fmt.Println("没有待补的书。")
		return
	}
	fmt.Printf("待补清单：%d 本（按字数降序），前 10 本的样例：\n", len(cands))
	for i, c := range cands {
		if i >= 10 {
			break
		}
		fmt.Printf("  %d · %s · 作者 %q · 简介 %d 字 · 分类 %q\n",
			c.BookID, c.Title, c.Author, len([]rune(c.Intro)), enrich.JoinCategory(c.CategoryL1, c.CategoryL2))
	}
	fmt.Println("下一步：加 --apply 真跑，或先用 --limit 100 小批量验证。")
}

// printEnrichReport 打印汇总表：扫描 / 命中 / 写入 / 跳过原因 / 请求数 / 熔断源 / 耗时。
func printEnrichReport(rep enrichReport, apply bool) {
	fmt.Printf("\n扫描 %d 本 / 命中 %d 本 / 变更 %d 条 / 写库 %d 条 · 请求 %d 次 · 用时 %s%s\n",
		rep.Scanned, rep.HitBooks, len(rep.Changes), rep.Written, rep.Engine.Requests,
		rep.Elapsed.Round(time.Second), applyHint(apply))
	if len(rep.Engine.Tripped) > 0 {
		fmt.Printf("熔断源：%v（连续失败已达阈值，本次不再请求）\n", rep.Engine.Tripped)
	}
	if len(rep.Skipped) == 0 {
		return
	}
	fmt.Println("跳过原因：")
	for _, k := range sortedSkipKeys(rep.Skipped) {
		fmt.Printf("  %-28s %d\n", k, rep.Skipped[k])
	}
	if !apply {
		fmt.Println("这是 dry-run：以上改动**没有**写库；加 --apply 才真写（并生成可回滚的 batch）。")
	}
}

// applyHint 返回写库状态提示。
func applyHint(apply bool) string {
	if apply {
		return "（已写库）"
	}
	return "（dry-run，未写库）"
}
