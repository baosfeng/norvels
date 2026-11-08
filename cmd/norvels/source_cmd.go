package main

// `norvels source test`：单源实测一次，回答「这个源到底能不能抽出书名和作者」。
//
//	norvels source test -db <库路径> [-id <src-xxxx>] [-title <书名>] [-json]
//
// 不带 -id 列出库里的在线书源；带 -id + -title 对该源发一次搜索请求。走的是
// internal/crawl 的同一条链路（header / URL 选项块 / 解码 / 规则求值），所以结果
// 与 enrich 的实际行为一致；候选不做相似度过滤，相似度原样打印，便于区分
// 「源坏了」与「只是搜到同人」。只读打开库，不写任何数据。

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"norvels/internal/crawl"
	"norvels/internal/enrich"
	"norvels/internal/source"
	"norvels/internal/store"
)

// sourceTestOpts 是 source test 的参数集合（CLI 与测试共用）。
type sourceTestOpts struct {
	dbPath  string
	id      string
	title   string
	jsonOut bool
	// options 透传给 crawl.NewEngine；CLI 用默认值，测试注入 dialer/端口。
	options crawl.Options
	out     io.Writer
}

// cmdSource 是 `norvels source` 的入口：目前只有 test 一个子命令。
func cmdSource(args []string) error {
	if len(args) == 0 || args[0] != "test" {
		return fmt.Errorf("用法：norvels source test -db <库路径> [-id <src-xxxx>] [-title <书名>] [-json]")
	}
	cfg := mustConfig()
	fs := flag.NewFlagSet("source test", flag.ExitOnError)
	o := sourceTestOpts{out: os.Stdout}
	fs.StringVar(&o.dbPath, "db", cfg.DB, "数据库路径")
	fs.StringVar(&o.id, "id", "", "书源 id（src-xxxx）")
	fs.StringVar(&o.title, "title", "", "要搜索的书名")
	fs.BoolVar(&o.jsonOut, "json", false, "输出结构化 JSON（脚本消费）")
	fs.Parse(args[1:])

	ctx := context.Background()
	st, err := store.OpenReadOnly(ctx, o.dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck // CLI 退出路径
	profiles, err := st.ListSourceProfiles(ctx)
	if err != nil {
		return err
	}
	return runSourceTest(ctx, profiles, o)
}

// runSourceTest 分派两种形态：只列书源，或对指定书源做一次自检。
func runSourceTest(ctx context.Context, profiles []source.Profile, o sourceTestOpts) error {
	online := onlineProfiles(profiles)
	if len(online) == 0 {
		return fmt.Errorf("库里没有在线书源（先导入 Legado 书源）")
	}
	if o.id == "" {
		return printSourceList(o.out, online, o.jsonOut)
	}
	return probeSource(ctx, online, o)
}

// probeSource 对指定书源发一次搜索并打印结果；失败时打印已拿到的信息后返回错误。
func probeSource(ctx context.Context, online []source.Profile, o sourceTestOpts) error {
	if strings.TrimSpace(o.title) == "" {
		return fmt.Errorf("--id 需要配合 --title 使用（要搜的书名）")
	}
	prof, ok := findOnlineProfile(online, o.id)
	if !ok {
		return fmt.Errorf("找不到书源 id=%q（可用：%s）", o.id, strings.Join(onlineIDs(online), "、"))
	}
	// 自检测的是「这份配置」，不受 enabled 开关影响（停用的源也要能先验证再启用）。
	prof.Enabled = true
	engine, err := crawl.NewEngine([]source.Profile{prof}, o.options)
	if err != nil {
		return err
	}
	probe, perr := engine.Probe(ctx, o.id, o.title)
	if probe == nil {
		return perr
	}
	if o.jsonOut {
		printProbeJSON(o.out, probe, perr)
	} else {
		printProbeText(o.out, probe, perr)
	}
	return perr
}

// onlineProfiles 过滤出可检索的在线书源（本地规则包不参与）。
func onlineProfiles(profiles []source.Profile) []source.Profile {
	out := make([]source.Profile, 0, len(profiles))
	for _, p := range profiles {
		if p.Kind == source.KindOnline && p.Online != nil {
			out = append(out, p)
		}
	}
	return out
}

// findOnlineProfile 按 id 取书源配置。
func findOnlineProfile(profiles []source.Profile, id string) (source.Profile, bool) {
	for _, p := range profiles {
		if p.ID == id {
			return p, true
		}
	}
	return source.Profile{}, false
}

// onlineIDs 返回全部在线书源 id（错误消息里给用户可选项）。
func onlineIDs(profiles []source.Profile) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.ID)
	}
	return out
}

// sourceInfoJSON 是列出模式的单条 JSON。
type sourceInfoJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// sourceListJSON 是列出模式的 JSON 包装。
type sourceListJSON struct {
	Sources []sourceInfoJSON `json:"sources"`
}

// printSourceList 输出书源 id 与名字（-json 时输出结构化结果）。
func printSourceList(w io.Writer, profiles []source.Profile, jsonOut bool) error {
	if jsonOut {
		list := sourceListJSON{Sources: make([]sourceInfoJSON, 0, len(profiles))}
		for _, p := range profiles {
			list.Sources = append(list.Sources, sourceInfoJSON{ID: p.ID, Name: p.Name, Enabled: p.Enabled})
		}
		return writeJSON(w, list)
	}
	for _, p := range profiles {
		suffix := ""
		if !p.Enabled {
			suffix = "（已停用）"
		}
		fmt.Fprintf(w, "%s  %s%s\n", p.ID, p.Name, suffix)
	}
	return nil
}

// probeJSON 是自检结果的 JSON 包装：失败时把错误一并放进结构里，stdout 始终可解析。
type probeJSON struct {
	crawl.Probe
	Error string `json:"error,omitempty"`
}

// printProbeJSON 输出结构化自检结果。
func printProbeJSON(w io.Writer, p *crawl.Probe, err error) {
	out := probeJSON{Probe: *p}
	if err != nil {
		out.Error = err.Error()
	}
	_ = writeJSON(w, out) // 写标准输出失败无法补救，保持与其它 CLI 一致
}

// writeJSON 以缩进 JSON 写一行结果。
func writeJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// printProbeText 输出人读的自检报告：请求、候选（带相似度）、降级与失败原因。
func printProbeText(w io.Writer, p *crawl.Probe, perr error) {
	fmt.Fprintf(w, "书源  %s  %s\n", p.SourceID, p.SourceName)
	fmt.Fprintf(w, "请求  GET %s  HTTP %d  耗时 %dms\n", p.URL, p.Status, p.ElapsedMS)
	threshold := enrich.DefaultOptions().MinTitleSim
	matched := countSimilar(p.Candidates, threshold)
	fmt.Fprintf(w, "候选  %d 条（相似度 ≥ %.2f 的 %d 条）\n", len(p.Candidates), threshold, matched)
	if len(p.Candidates) == 0 {
		fmt.Fprintln(w, "  （一条都没抽到：先看降级信息与列表规则选择器）")
	}
	for i, c := range p.Candidates {
		fmt.Fprintf(w, "  %2d  %.2f  %s / %s  %s\n", i+1, c.Similarity, c.Title, c.Author, c.BookURL)
	}
	if len(p.Degraded) > 0 {
		fmt.Fprintf(w, "降级  %s\n", strings.Join(p.Degraded, "；"))
	}
	if perr != nil {
		fmt.Fprintf(w, "失败  %v\n", perr)
	}
}

// countSimilar 统计相似度不低于阈值的候选数。
func countSimilar(cands []crawl.ProbeCandidate, threshold float64) int {
	n := 0
	for _, c := range cands {
		if c.Similarity >= threshold {
			n++
		}
	}
	return n
}
