package main

import (
	"strings"
	"testing"
)

func TestFindCyclesDetectsSelfContainedCycle(t *testing.T) {
	deps := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	cycles := findCycles([]string{"a", "b", "c"}, deps)
	if len(cycles) != 1 {
		t.Fatalf("环数 = %d，期望 1（%v）", len(cycles), cycles)
	}
	if got := strings.Join(cycles[0], ">"); got != "a>b>c>a" {
		t.Fatalf("环路径 = %q，期望 %q", got, "a>b>c>a")
	}
}

func TestFindCyclesNoCycle(t *testing.T) {
	deps := map[string][]string{
		"api":   {"store"},
		"store": {"model"},
		"model": {},
	}
	if cycles := findCycles([]string{"api", "store", "model"}, deps); len(cycles) != 0 {
		t.Fatalf("无环图返回了环：%v", cycles)
	}
}

func TestCheckDirectionAcceptsLayering(t *testing.T) {
	deps := map[string][]string{
		"norvels/internal/model": {},
		"norvels/internal/store": {"norvels/internal/model"},
	}
	got := checkDirection([]string{"norvels/internal/model", "norvels/internal/store"}, deps)
	if len(got) != 0 {
		t.Fatalf("合法分层被判违规：%v", got)
	}
}

func TestCheckDirectionRejectsReverseImport(t *testing.T) {
	deps := map[string][]string{
		"norvels/internal/store": {"norvels/internal/api"},
	}
	got := checkDirection([]string{"norvels/internal/store"}, deps)
	if len(got) == 0 {
		t.Fatalf("store → api 反向依赖未被判违规")
	}
	if !strings.Contains(got[0], "不在允许列表") {
		t.Fatalf("违规描述未说明原因：%q", got[0])
	}
}

func TestCheckDirectionRejectsUnregisteredPackage(t *testing.T) {
	deps := map[string][]string{
		"norvels/internal/brandnew": {},
	}
	got := checkDirection([]string{"norvels/internal/brandnew"}, deps)
	if len(got) == 0 {
		t.Fatalf("未登记的新包未被判违规")
	}
	if !strings.Contains(got[0], "未在 allowedDeps 中登记") {
		t.Fatalf("违规描述未说明原因：%q", got[0])
	}
}

func TestCheckDirectionRejectsCmdReverseImport(t *testing.T) {
	deps := map[string][]string{
		"norvels/internal/api": {"norvels/cmd/norvels"},
	}
	got := checkDirection([]string{"norvels/internal/api"}, deps)
	if len(got) == 0 {
		t.Fatalf("internal → cmd 反向依赖未被判违规")
	}
	if !strings.Contains(got[0], "反向依赖程序入口") {
		t.Fatalf("违规描述未说明原因：%q", got[0])
	}
}

func TestCheckDirectionRejectsToolDependingOnInternal(t *testing.T) {
	deps := map[string][]string{
		"norvels/tests/tools/gostats": {"norvels/internal/store"},
	}
	got := checkDirection([]string{"norvels/tests/tools/gostats"}, deps)
	if len(got) == 0 {
		t.Fatalf("门禁工具依赖 internal 未被判违规")
	}
}

func TestCheckDirectionAllowsAcceptancePackage(t *testing.T) {
	deps := map[string][]string{
		"norvels/tests/acceptance": {"norvels/internal/api"},
	}
	if got := checkDirection([]string{"norvels/tests/acceptance"}, deps); len(got) != 0 {
		t.Fatalf("验收测试包应允许依赖 internal：%v", got)
	}
}
