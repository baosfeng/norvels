package importer

import (
	"testing"

	"norvels/internal/source"
)

// mustCompile 编译一份规则包，失败即终止用例。
func mustCompile(t *testing.T, p source.Profile) Compiled {
	t.Helper()
	c, err := CompileProfile(p)
	if err != nil {
		t.Fatalf("CompileProfile(%q): %v", p.ID, err)
	}
	return c
}

// sampleCandidate 造一个普通文件候选。
func sampleCandidate(name string) candidate {
	return candidate{path: "/books/示例/" + name, source: "/books/示例/" + name, name: name}
}

// TestSelectByMatch match 的 ext 与 nameRegex 全「与」才认领。
func TestSelectByMatch(t *testing.T) {
	pc := mustCompile(t, profileFixture("sample"))
	sel := profileSelector{compiled: []Compiled{pc}}
	if got := sel.pick("", sampleCandidate("示例-a.txt")); got == nil || got.ProfileID != "sample" {
		t.Fatalf("命中名称规则应选上: %+v", got)
	}

	// 扩展名不符：规则包只收 txt。
	pcTxt := mustCompile(t, profileFixture("sample"))
	pcTxt.matchExts = []string{"epub"}
	if got := (profileSelector{compiled: []Compiled{pcTxt}}).pick("", sampleCandidate("示例-a.txt")); got != nil {
		t.Fatalf("扩展名不符不该命中: %+v", got)
	}

	// 名称不符：nameRegex 是 ^示例。
	if got := sel.pick("", sampleCandidate("别的书.txt")); got != nil {
		t.Fatalf("名称不符不该命中: %+v", got)
	}
}

// TestSelectPriority 多份都命中时取 priority 最高的一份（同优先级取配置顺序靠前）。
func TestSelectPriority(t *testing.T) {
	low := profileFixture("low")
	low.Match = source.Match{NameRegex: "^示例"}
	low.Priority = 1
	high := profileFixture("high")
	high.Match = source.Match{NameRegex: "^示例"}
	high.Priority = 99

	sel := profileSelector{compiled: []Compiled{
		mustCompile(t, low), mustCompile(t, high),
	}}
	// 直接构造的 selector 未按 priority 排序；pick 依赖调用方排序，这里先按
	// CompileProfiles 的语义排序，模拟真实入口。
	compiled, _ := CompileProfiles([]source.Profile{low, high})
	sel = profileSelector{compiled: compiled}
	if got := sel.pick("", sampleCandidate("示例-a.txt")); got == nil || got.ProfileID != "high" {
		t.Fatalf("应选 priority 最高的 high: %+v", got)
	}
}

// TestSelectExplicit 显式指定优先于 match（即使指定的那份优先级更低）。
func TestSelectExplicit(t *testing.T) {
	low := profileFixture("low")
	low.Match = source.Match{NameRegex: "^示例"}
	low.Priority = 1
	high := profileFixture("high")
	high.Match = source.Match{NameRegex: "^示例"}
	high.Priority = 99
	compiled, _ := CompileProfiles([]source.Profile{low, high})
	sel := profileSelector{compiled: compiled}

	if got := sel.pick("low", sampleCandidate("示例-a.txt")); got == nil || got.ProfileID != "low" {
		t.Fatalf("显式指定应生效: %+v", got)
	}
	if got := sel.pick("missing", sampleCandidate("示例-a.txt")); got != nil {
		t.Fatalf("显式指定不存在的 id 应返回 nil（由调用方报错）: %+v", got)
	}
}

// TestSelectNoMatch 没有任何规则包认领时返回 nil（走内置策略）。
func TestSelectNoMatch(t *testing.T) {
	sel := profileSelector{compiled: []Compiled{mustCompile(t, profileFixture("sample"))}}
	if got := sel.pick("", sampleCandidate("无关文件.txt")); got != nil {
		t.Fatalf("无命中应返回 nil: %+v", got)
	}
}

// TestSelectZipEntry zip 条目用包名匹配 zipNameRegex、包内名匹配 nameRegex。
func TestSelectZipEntry(t *testing.T) {
	p := profileFixture("zip")
	p.Match = source.Match{ZipNameRegex: "^合订包\\.zip$", NameRegex: "^示例"}
	sel := profileSelector{compiled: []Compiled{mustCompile(t, p)}}

	hit := candidate{
		path:   "/tmp/unzip/示例-a.txt",
		source: "/books/合订包.zip:示例-a.txt",
		name:   "合订包.zip:示例-a.txt",
	}
	if got := sel.pick("", hit); got == nil {
		t.Fatal("zip 条目应命中")
	}
	// 普通文件没有包名，zipNameRegex 不命中。
	if got := sel.pick("", sampleCandidate("示例-a.txt")); got != nil {
		t.Fatalf("普通文件不该被 zipNameRegex 认领: %+v", got)
	}
}

// TestGlobMatchRecursive /books/示例/** 按前缀递归，其余交给 path.Match。
func TestGlobMatchRecursive(t *testing.T) {
	if !globMatch("/books/示例/**", "/books/示例/子目录/书.txt") {
		t.Error("/** 应递归匹配子目录")
	}
	if globMatch("/books/示例/**", "/books/其他/书.txt") {
		t.Error("/** 不该匹配前缀之外")
	}
	if !globMatch("/books/示例/*.txt", "/books/示例/书.txt") {
		t.Error("单星号应匹配同层文件")
	}
	if globMatch("/books/示例/*.txt", "/books/示例/子/书.txt") {
		t.Error("单星号不该跨目录")
	}
}

// TestCandidateNameParts zip 条目的文件名与包名拆分。
func TestCandidateNameParts(t *testing.T) {
	zipCand := candidate{name: "合订包.zip:子目录/示例-a.txt"}
	if got := candidateFileName(zipCand); got != "示例-a.txt" {
		t.Errorf("文件名 = %q，期望 示例-a.txt", got)
	}
	if got := zipPackageName(zipCand); got != "合订包.zip" {
		t.Errorf("包名 = %q，期望 合订包.zip", got)
	}
	plain := candidate{name: "示例-a.txt"}
	if got := zipPackageName(plain); got != "" {
		t.Errorf("普通文件包名 = %q，期望空串", got)
	}
}
