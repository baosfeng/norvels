// Command corpusstats 统计全库正文字符频次，作为「全局字码本」的码表依据。
//
// 为什么写成一次性工具而不是 internal/store 的能力：它要跑在生产库上、只读、
// 只此一次，并且必须能交叉编译成单文件丢到 NAS 上跑（NAS 上没有 Go 工具链）。
// 所以解压逻辑在本包内自带一份（与 internal/store/compress.go 的格式约定一致），
// 不 import internal/store——工具越独立越好，也避免误碰服务端读路径。
//
// 用法：
//
//	corpusstats -db <path> -out <dir> -all
//	corpusstats -db <path> -out <dir> -sample 50
//
// 产物（写到 -out 目录，约定放仓库外）：
//
//	corpus-freq.tsv    <codepoint 十进制> TAB <count>，按 count 降序
//	corpus-stats.json  total_rune / distinct_rune / invalid_bytes / blocks / algo_histogram
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "corpusstats: %v\n", err)
		os.Exit(1)
	}
}

// config 是一次运行的参数。
type config struct {
	db      string
	out     string
	sample  int
	all     bool
	workers int
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	db, err := openReadOnly(cfg.db)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	p, err := planBlocks(db, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "corpusstats: 模式=%s 书=%d 块=%d workers=%d\n",
		p.mode, p.books, p.blocks, cfg.workers)

	agg, err := countAll(db, p, cfg.workers)
	if err != nil {
		return err
	}
	if err := writeOutputs(cfg.out, p, agg); err != nil {
		return err
	}
	return printSummary(p, agg)
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("corpusstats", flag.ContinueOnError)
	fs.StringVar(&cfg.db, "db", "", "SQLite 库路径（只读打开）")
	fs.StringVar(&cfg.out, "out", "", "输出目录（放仓库外）")
	fs.IntVar(&cfg.sample, "sample", 0, "按 book_id 等间隔抽样的书本数")
	fs.BoolVar(&cfg.all, "all", false, "统计全库")
	fs.IntVar(&cfg.workers, "workers", runtime.NumCPU(), "并发解压 worker 数")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.db == "" || cfg.out == "" {
		return cfg, errors.New("-db 与 -out 都是必填")
	}
	if cfg.all == (cfg.sample > 0) {
		return cfg, errors.New("-all 与 -sample N 必须且只能给一个")
	}
	if cfg.workers < 1 {
		cfg.workers = 1
	}
	return cfg, nil
}

// writeOutputs 落两个产物：TSV 给码表用，JSON 记口径与规模。
func writeOutputs(dir string, p plan, c *counter) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := writeFreq(filepath.Join(dir, "corpus-freq.tsv"), c); err != nil {
		return err
	}
	return writeStats(filepath.Join(dir, "corpus-stats.json"), p, c)
}

// statsJSON 是 corpus-stats.json 的字段（契约字段名不可改，下游按名取值）。
type statsJSON struct {
	TotalRune       uint64            `json:"total_rune"`
	DistinctRune    int               `json:"distinct_rune"`
	InvalidBytes    uint64            `json:"invalid_bytes"`
	RawBytes        uint64            `json:"raw_bytes"`
	Blocks          uint64            `json:"blocks"`
	AlgoHistogram   map[string]uint64 `json:"algo_histogram"`
	InvalidByteHist map[string]uint64 `json:"invalid_byte_histogram"`
	Books           int               `json:"books"`
	Mode            string            `json:"mode"`
}

// writeFreq 写码表输入：code<TAB>count，按 count 降序。
func writeFreq(path string, c *counter) error {
	entries := c.entries()
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建 %s 失败: %w", path, err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%d\t%d\n", e.r, e.n); err != nil {
			_ = f.Close()
			return fmt.Errorf("写 %s 失败: %w", path, err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("刷 %s 失败: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关闭 %s 失败: %w", path, err)
	}
	return nil
}

// writeStats 写口径文件。
func writeStats(path string, p plan, c *counter) error {
	entries := c.entries()
	payload := statsJSON{
		TotalRune:       c.runes,
		DistinctRune:    len(entries),
		InvalidBytes:    c.badTotal,
		RawBytes:        c.rawBytes,
		Blocks:          c.blocks,
		AlgoHistogram:   c.algoNames(),
		InvalidByteHist: c.badNames(),
		Books:           p.books,
		Mode:            p.mode,
	}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化统计失败: %w", err)
	}
	if err := os.WriteFile(path, append(buf, '\n'), 0o644); err != nil {
		return fmt.Errorf("写 %s 失败: %w", path, err)
	}
	return nil
}

// printSummary 在 stdout 打一段人读摘要（合计 + top-20），供人工核对。
func printSummary(p plan, c *counter) error {
	entries := c.entries()
	var sum uint64
	for _, e := range entries {
		sum += e.n
	}
	pct := func(n uint64) float64 {
		if sum == 0 {
			return 0
		}
		return float64(n) * 100 / float64(sum)
	}
	fmt.Fprintf(os.Stdout, "模式=%s 书=%d 块=%d total_rune=%d distinct_rune=%d invalid_bytes=%d\n",
		p.mode, p.books, c.blocks, c.runes, len(entries), c.badTotal)
	top := 20
	if len(entries) < top {
		top = len(entries)
	}
	for i := 0; i < top; i++ {
		e := entries[i]
		fmt.Fprintf(os.Stdout, "%2d  U+%04X %q %d %.3f%%\n", i+1, e.r, string(e.r), e.n, pct(e.n))
	}
	return nil
}
