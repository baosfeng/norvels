package importer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"norvels/internal/parser"
	"norvels/internal/source"
)

// profileDir 造「同目录两份命名各命中不同规则包」的语料：甲用分节符切成 2 章，
// 乙没有专属切章规则、走内置切章（3 章）。
func profileDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTxt(t, dir, "示例-甲.txt",
		"书名：示例甲\n作者：示例作者\n\n=== 第1 节 ===\n正文甲第一段。\n=== 第2 节 ===\n正文甲第二段。\n")
	writeTxt(t, dir, "示例-乙.txt", sampleText("示例乙", "示例作者"))
	return dir
}

// profilePair 返回示例-甲 / 示例-乙 两份规则包（全虚构）。
func profilePair() []source.Profile {
	jia := source.Profile{
		ID: "jia", Name: "甲来源", SchemaVersion: 1, Enabled: true, Priority: 10,
		Match: source.Match{Ext: []string{"txt"}, NameRegex: "^示例-甲"},
		Split: source.Split{Rules: []source.Rule{{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"}}},
	}
	yi := source.Profile{
		ID: "yi", Name: "乙来源", SchemaVersion: 1, Enabled: true, Priority: 10,
		Match: source.Match{Ext: []string{"txt"}, NameRegex: "^示例-乙"},
	}
	return []source.Profile{jia, yi}
}

// sampleByBase 把样例按文件名索引。
func sampleByBase(rep *Report) map[string]ReportSample {
	out := make(map[string]ReportSample, len(rep.Sample))
	for _, s := range rep.Sample {
		out[filepath.Base(s.Source)] = s
	}
	return out
}

// TestDryRunAppliesProfilePerName 两份命名各命中不同规则包：profileId 与章数都不同。
func TestDryRunAppliesProfilePerName(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := profileDir(t)
	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true,
		ParserOpts: parser.DefaultOptions(), Profiles: profilePair()})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(rep.Sample) != 2 {
		t.Fatalf("样例数 = %d，期望 2", len(rep.Sample))
	}
	got := sampleByBase(rep)
	if s := got["示例-甲.txt"]; s.ProfileID != "jia" || s.Chapters != 2 {
		t.Errorf("示例-甲 = profileId:%q chapters:%d，期望 jia/2", s.ProfileID, s.Chapters)
	}
	if s := got["示例-乙.txt"]; s.ProfileID != "yi" || s.Chapters != 3 {
		t.Errorf("示例-乙 = profileId:%q chapters:%d，期望 yi/3", s.ProfileID, s.Chapters)
	}
}

// TestDryRunExplicitProfile 显式 profileId：所有文件都走这一份；规则零命中时退回内置切章。
func TestDryRunExplicitProfile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := profileDir(t)
	rep, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true,
		ParserOpts: parser.DefaultOptions(), Profiles: profilePair(), ProfileID: "jia"})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	for _, s := range rep.Sample {
		if s.ProfileID != "jia" {
			t.Errorf("%s 的 profileId = %q，期望 jia", filepath.Base(s.Source), s.ProfileID)
		}
	}
	if s := sampleByBase(rep)["示例-乙.txt"]; s.Chapters != 3 {
		t.Errorf("乙的章数 = %d，期望退回内置切章后的 3", s.Chapters)
	}
}

// TestDryRunUnknownExplicitProfile 未配置的 id → ErrProfileNotFound（api 映射 400）。
func TestDryRunUnknownExplicitProfile(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := profileDir(t)
	_, err := DryRun(ctx, st, Options{Dir: dir, Recursive: true,
		ParserOpts: parser.DefaultOptions(), Profiles: profilePair(), ProfileID: "missing"})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("err = %v，期望 ErrProfileNotFound", err)
	}
}

// TestRunAppliesProfileAndReports 真实导入：规则包生效，Result 记录每个文件的 profileId。
func TestRunAppliesProfileAndReports(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := profileDir(t)
	res, err := Run(ctx, st, Options{Dir: dir, Recursive: true,
		ParserOpts: parser.DefaultOptions(), Profiles: profilePair()}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("入库数 = %d，期望 2", len(res.Imported))
	}
	applied := make(map[string]string, len(res.AppliedProfiles))
	for _, ap := range res.AppliedProfiles {
		applied[filepath.Base(ap.Source)] = ap.ProfileID
	}
	if applied["示例-甲.txt"] != "jia" || applied["示例-乙.txt"] != "yi" {
		t.Errorf("AppliedProfiles = %v，期望 甲=jia 乙=yi", applied)
	}
}
