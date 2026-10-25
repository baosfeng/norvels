package main

import (
	"flag"
	"strings"
	"testing"

	"norvels/internal/store"
)

// TestPrintContentCheck 覆盖 check-content 的输出格式。
//
// 这段输出是运维在迁移后看的第一手证据，字段缺失会让人无法判断
// 「正文到底读出来了没有、块切得合不合理」。
func TestPrintContentCheck(t *testing.T) {
	rep := store.ContentCheckReport{
		TotalChapters: 100,
		Sampled:       50,
		EmptyChapters: 2,
		MinBytes:      100,
		MaxBytes:      20000,
		TotalBytes:    250000,
		Blocks:        12,
	}
	out := captureStdout(t, func() { printContentCheck(rep, false) })
	for _, want := range []string{"抽查 50/100 章", "12 个压缩块", "成功读出 50 章", "250000", "100", "20000", "平均 5000", "空章 2 个"} {
		if !containsStr(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
}

// TestPrintContentCheckAllMode 全量模式要把「全量校验」打出来，
// 让运维一眼确认自己跑的是全量而不是抽查。
func TestPrintContentCheckAllMode(t *testing.T) {
	rep := store.ContentCheckReport{TotalChapters: 100, Sampled: 100, MinBytes: 10, MaxBytes: 20, TotalBytes: 1500, Blocks: 3}
	out := captureStdout(t, func() { printContentCheck(rep, true) })
	if !containsStr(out, "全量校验 100/100 章") {
		t.Errorf("全量模式输出不符合预期：\n%s", out)
	}
}

// TestAvgBytes 覆盖平均值的零除保护。
func TestAvgBytes(t *testing.T) {
	if got := avgBytes(100, 0); got != 0 {
		t.Errorf("avgBytes(100, 0) = %d，期望 0", got)
	}
	if got := avgBytes(100, 4); got != 25 {
		t.Errorf("avgBytes(100, 4) = %d，期望 25", got)
	}
}

// TestContentCheckOptionsMutualExclusion --all 与 --sample 同时给必须报错。
//
// 不能「--all 悄悄赢」：运维敲 --all --sample 10 时意图有歧义，
// 猜错会让人误以为全量跑过了。
func TestContentCheckOptionsMutualExclusion(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"只给 --all", []string{"--all"}, false},
		{"只给 --sample", []string{"--sample", "10"}, false},
		{"都不给", nil, false},
		{"两个都给", []string{"--all", "--sample", "10"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := flag.NewFlagSet("check-content", flag.ContinueOnError)
			all := fs.Bool("all", false, "")
			sample := fs.Int("sample", defaultCheckSample, "")
			if err := fs.Parse(c.args); err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, err := contentCheckOptions(*all, *sample, fs)
			if c.wantErr && err == nil {
				t.Fatal("互斥参数应报错")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("不该报错: %v", err)
			}
			if c.wantErr && err != nil && !strings.Contains(err.Error(), "互斥") {
				t.Errorf("错误信息应说明互斥：%v", err)
			}
		})
	}
}

// TestContentCheckOptionsSelectsAll 只给 --all 时应传达 All=true。
func TestContentCheckOptionsSelectsAll(t *testing.T) {
	fs := flag.NewFlagSet("check-content", flag.ContinueOnError)
	all := fs.Bool("all", false, "")
	sample := fs.Int("sample", defaultCheckSample, "")
	if err := fs.Parse([]string{"--all"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	opt, err := contentCheckOptions(*all, *sample, fs)
	if err != nil {
		t.Fatalf("contentCheckOptions: %v", err)
	}
	if !opt.All {
		t.Error("--all 应设置 All=true")
	}
}

// TestContentCheckResultListsAllBlocks 有坏块时输出必须列出全部，并返回错误。
func TestContentCheckResultListsAllBlocks(t *testing.T) {
	rep := store.ContentCheckReport{
		TotalChapters: 100,
		Sampled:       100,
		MinBytes:      10,
		MaxBytes:      20,
		TotalBytes:    1500,
		Blocks:        3,
		Corrupted: []*store.BlockError{
			{BlockID: 1, BookID: 7, Seq: 1, FirstIdx: 0, LastIdx: 34, ChapterID: 5},
			{BlockID: 2, BookID: 9, Seq: 2, FirstIdx: 35, LastIdx: 70, ChapterID: 40},
		},
	}
	var err error
	out := captureStdout(t, func() { err = contentCheckResult(rep) })
	if err == nil {
		t.Fatal("有坏块时应返回错误")
	}
	if !containsStr(out, "发现 2 个损坏的压缩块") {
		t.Errorf("应报出损坏块总数：\n%s", out)
	}
	for _, want := range []string{"book_id=7", "book_id=9", "block_seq=1", "block_seq=2"} {
		if !containsStr(out, want) {
			t.Errorf("清单应包含 %q：\n%s", want, out)
		}
	}
}

// TestContentCheckResultOK 无损坏时输出 OK 且不返回错误。
func TestContentCheckResultOK(t *testing.T) {
	rep := store.ContentCheckReport{TotalChapters: 10, Sampled: 10, MinBytes: 5, MaxBytes: 9, TotalBytes: 70, Blocks: 2}
	var err error
	out := captureStdout(t, func() { err = contentCheckResult(rep) })
	if err != nil {
		t.Fatalf("正常库不应返回错误: %v", err)
	}
	if !containsStr(out, "OK") {
		t.Errorf("应输出 OK：\n%s", out)
	}
}

// TestContentCheckResultBadUTF8IsNoticeNotFailure 非法 UTF-8 只提示，不判失败。
//
// 真实库踩过：迁移逐字节保住了源文件里的 20 章非法 UTF-8（这是正确行为），
// 但旧逻辑报 FAIL + exit 1，让用户以为迁移把数据搞坏了。
func TestContentCheckResultBadUTF8IsNoticeNotFailure(t *testing.T) {
	rep := store.ContentCheckReport{
		TotalChapters:     10,
		Sampled:           10,
		BadUTF8:           2,
		BadUTF8ChapterIDs: []int64{7, 8},
	}
	var err error
	out := captureStdout(t, func() { err = contentCheckResult(rep) })
	if err != nil {
		t.Fatalf("非法 UTF-8 不应判失败（存储是完好的）: %v", err)
	}
	for _, want := range []string{"提示：2 章正文含非法 UTF-8", "源文件编码问题", "非迁移损坏", "[7 8]", "OK"} {
		if !containsStr(out, want) {
			t.Errorf("输出缺少 %q：\n%s", want, out)
		}
	}
	// 绝不能出现把责任推给存储/迁移的措辞。
	if containsStr(out, "FAIL") {
		t.Errorf("不应出现 FAIL：\n%s", out)
	}
	if containsStr(out, "解压或迁移出错") {
		t.Errorf("措辞误导（甩锅给存储层）：\n%s", out)
	}
}
