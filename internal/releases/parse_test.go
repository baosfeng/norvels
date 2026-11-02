package releases

import "testing"

// 说明文件的解析规则：版本标题 + 四个中文章节名 + "- " 条目。
// 这里用合成文本（不碰真实语料），只为固定格式与失败口径。

const sampleMD = `# 发版说明

## [0.2.0] - 2026-01-02 （据 git 历史回溯）

### 新增
- 甲功能上线
- 乙功能上线
### 改进
- 丙更顺手
### 修复
- 丁不再出错
### 升级注意
- 先备份

## [0.1.0] - 2026-01-01

### 新增
- 首个版本
`

func TestParseGroups(t *testing.T) {
	items, err := parse([]byte(sampleMD))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("版本数 = %d，期望 2", len(items))
	}
	if items[0].Version != "0.2.0" || items[0].Date != "2026-01-02" {
		t.Fatalf("版本头解析错: %+v", items[0])
	}
	first := items[0]
	if !first.Retro {
		t.Error("带回溯标记的版本应标 Retro")
	}
	if items[1].Retro {
		t.Error("没有回溯标记的版本不应标 Retro")
	}
	if len(first.Sections.Added) != 2 || first.Sections.Added[1] != "乙功能上线" {
		t.Errorf("新增 = %v", first.Sections.Added)
	}
	requireEachOne(t, first)
}

// requireEachOne 断言「改进 / 修复 / 升级注意」各有一条：三分组都是数量断言，
// 分开写比塞进同一个测试里的判定表更好读。
func requireEachOne(t *testing.T, e Entry) {
	t.Helper()
	groups := map[string][]string{
		"改进": e.Sections.Changed, "修复": e.Sections.Fixed, "升级注意": e.Sections.Upgrade,
	}
	for name, items := range groups {
		if len(items) != 1 {
			t.Errorf("%s 条数 = %d，期望 1", name, len(items))
		}
	}
}

func TestParseFailsClosed(t *testing.T) {
	cases := []struct{ name, md string }{
		{"空文件", ""},
		{"条目没有版本", "### 新增\n- 甲\n"},
		{"未知分组", "## [0.1.0] - 2026-01-01\n### 重构\n- 甲\n"},
		{"条目没有分组", "## [0.1.0] - 2026-01-01\n- 甲\n"},
		{"版本没有任何条目", "## [0.1.0] - 2026-01-01\n### 新增\n"},
		{"版本标题格式错", "## 0.1.0 首版\n### 新增\n- 甲\n"},
		{"无法识别的行", "## [0.1.0] - 2026-01-01\n### 新增\n改为一段散文\n"},
	}
	for _, c := range cases {
		if _, err := parse([]byte(c.md)); err == nil {
			t.Errorf("%s：期望报错，实际通过", c.name)
		}
	}
}

func TestParseContinuationLine(t *testing.T) {
	md := "## [0.1.0] - 2026-01-01\n### 升级注意\n- 先备份\n  再升级\n"
	items, err := parse([]byte(md))
	if err != nil {
		t.Fatalf("缩进续行应可解析: %v", err)
	}
	if got := items[0].Sections.Upgrade[0]; got != "先备份再升级" {
		t.Fatalf("续行未接到上一条: %q", got)
	}
}

func TestBuildOrdersAndMarksCurrent(t *testing.T) {
	md := []byte(`## [0.9.0] - 2026-03-01
### 新增
- 新
## [0.10.0] - 2026-04-01
### 新增
- 更新
## [0.9.10] - 2026-02-01
### 修复
- 更旧但补丁号大
`)
	notes, err := Build(md, "0.10.0")
	if err != nil {
		t.Fatalf("Build 失败: %v", err)
	}
	var order []string
	for _, e := range notes.Items {
		order = append(order, e.Version)
	}
	want := []string{"0.10.0", "0.9.10", "0.9.0"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("顺序 = %v，期望 %v", order, want)
		}
	}
	if notes.Current != "0.10.0" {
		t.Fatalf("current = %q", notes.Current)
	}
}

func TestBuildCurrentMissing(t *testing.T) {
	md := []byte("## [0.1.0] - 2026-01-01\n### 新增\n- 甲\n")
	for _, cur := range []string{"", "0.2.0", "dev"} {
		notes, err := Build(md, cur)
		if err != nil {
			t.Fatalf("Build(%q) 失败: %v", cur, err)
		}
		if notes.Current != "" {
			t.Errorf("Build(%q).current = %q，期望空", cur, notes.Current)
		}
	}
}

func TestParseAcceptsEnglishGroupAliases(t *testing.T) {
	md := []byte("## [0.1.0] - 2026-01-01\n### Added\n- 甲\n### Upgrade\n- 乙\n")
	items, err := parse(md)
	if err != nil {
		t.Fatalf("英文分组别名为兼容写法，应可解析: %v", err)
	}
	if len(items[0].Sections.Added) != 1 || len(items[0].Sections.Upgrade) != 1 {
		t.Fatalf("分组 = %+v", items[0].Sections)
	}
}

func TestSectionsEmptyIsNotNil(t *testing.T) {
	md := []byte("## [0.1.0] - 2026-01-01\n### 新增\n- 甲\n")
	items, err := parse(md)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if items[0].Sections.Fixed == nil || items[0].Sections.Upgrade == nil {
		t.Fatal("空分组必须是空切片（JSON 给 [] 而不是 null）")
	}
}
