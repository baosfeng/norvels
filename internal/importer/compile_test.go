package importer

import (
	"strings"
	"testing"

	"norvels/internal/source"
)

// profileFixture 返回一份覆盖各层的规则包（示例全虚构）。
func profileFixture(id string) source.Profile {
	return source.Profile{
		ID: id, Name: "示例来源", SchemaVersion: 1, Enabled: true, Priority: 10,
		Match:  source.Match{Ext: []string{"txt"}, NameRegex: "^示例"},
		Decode: source.Decode{ForceEncoding: "gb18030"},
		Filters: source.Filters{
			Disable: []string{"width"},
			Extra:   []source.FilterRule{{Name: "广告行", Priority: 30, Pattern: "^广告：.*$", Replace: ""}},
		},
		Meta:   []source.MetaRule{{Name: "头部作者", Weight: 60, Pattern: "^作者：(.+)$", Author: 1}},
		Split:  source.Split{ForceSplitter: "di-unit", Rules: []source.Rule{{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"}}},
		Tags:   []source.TagRule{{Tag: "奇幻", Keywords: []string{"魔法"}}},
		Limits: source.Limits{MaxChapterChars: 12345, MinChapters: 2},
	}
}

// TestCompileProfileMapsFields 规则包各层字段落到 parser.Options / Rule / TagRule。
func TestCompileProfileMapsFields(t *testing.T) {
	c, err := CompileProfile(profileFixture("sample"))
	if err != nil {
		t.Fatalf("CompileProfile: %v", err)
	}
	requireCompiledIdentity(t, &c)
	requireCompiledLayers(t, &c)
}

// requireCompiledIdentity 断言身份与 limits 的映射。
func requireCompiledIdentity(t *testing.T, c *Compiled) {
	t.Helper()
	if c.ProfileID != "sample" || c.Priority != 10 {
		t.Errorf("id/priority = %q/%d", c.ProfileID, c.Priority)
	}
	if c.Opts.MaxChapterChars != 12345 || c.MinChapters != 2 {
		t.Errorf("limits = maxChapterChars:%d minChapters:%d", c.Opts.MaxChapterChars, c.MinChapters)
	}
}

// requireCompiledLayers 断言编码 / 过滤 / 元数据 / 切章 / 标签各层的映射。
func requireCompiledLayers(t *testing.T, c *Compiled) {
	t.Helper()
	requireCompiledDecode(t, c)
	requireCompiledSplit(t, c)
	if len(c.Tags) != 1 || c.Tags[0].Tag != "奇幻" {
		t.Errorf("tags = %v", c.Tags)
	}
}

// requireCompiledDecode 断言编码与过滤层。
func requireCompiledDecode(t *testing.T, c *Compiled) {
	t.Helper()
	if c.Opts.ForceEncoding != "gb18030" {
		t.Errorf("forceEncoding = %q", c.Opts.ForceEncoding)
	}
	if len(c.Opts.DisableFilters) != 1 || c.Opts.DisableFilters[0] != "width" {
		t.Errorf("disableFilters = %v", c.Opts.DisableFilters)
	}
	if len(c.Opts.ExtraFilters) != 1 || c.Opts.ExtraFilters[0].Name() != "广告行" {
		t.Errorf("extraFilters = %v", c.Opts.ExtraFilters)
	}
}

// requireCompiledSplit 断言元数据与切章层。
func requireCompiledSplit(t *testing.T, c *Compiled) {
	t.Helper()
	if len(c.Opts.ExtraMetas) != 1 || c.Opts.ExtraMetas[0].Weight() != 60 {
		t.Errorf("extraMetas = %v", c.Opts.ExtraMetas)
	}
	if c.Opts.ForceSplitter != "di-unit" {
		t.Errorf("forceSplitter = %q", c.Opts.ForceSplitter)
	}
	if len(c.Rules) != 1 || c.Rules[0].Name != "分节符" {
		t.Errorf("切章规则 = %v", c.Rules)
	}
}

// TestCompileProfileRejects 坏编码名 / 坏策略名 / 坏正则都在编译期拒绝。
func TestCompileProfileRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*source.Profile)
		want   string
	}{
		{"未知编码", func(p *source.Profile) { p.Decode.ForceEncoding = "no-such-codec" }, "forceEncoding"},
		{"未知切章策略", func(p *source.Profile) { p.Split.ForceSplitter = "no-such-splitter" }, "forceSplitter"},
		{"坏过滤正则", func(p *source.Profile) { p.Filters.Extra[0].Pattern = "^(" }, "过滤"},
		{"坏元数据正则", func(p *source.Profile) { p.Meta[0].Pattern = "^(" }, "元数据"},
		{"坏切章正则", func(p *source.Profile) { p.Split.Rules[0].Pattern = "^(" }, "切章规则"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := profileFixture("sample")
			tc.mutate(&p)
			if _, err := CompileProfile(p); err == nil {
				t.Fatalf("应被拒绝")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误 %q 不含 %q", err.Error(), tc.want)
			}
		})
	}
}

// TestCompileProfilesSkipsBadAndSorts 坏包整份跳过并记 warning；按 priority 降序。
func TestCompileProfilesSkipsBadAndSorts(t *testing.T) {
	bad := profileFixture("bad")
	bad.Decode.ForceEncoding = "no-such-codec"
	low := profileFixture("low")
	low.Priority = 1
	high := profileFixture("high")
	high.Priority = 99

	compiled, warnings := CompileProfiles([]source.Profile{bad, low, high})
	if len(compiled) != 2 || len(warnings) != 1 {
		t.Fatalf("compiled/warnings = %d/%d，期望 2/1: %v", len(compiled), len(warnings), warnings)
	}
	if compiled[0].ProfileID != "high" || compiled[1].ProfileID != "low" {
		t.Errorf("排序 = %q, %q，期望 high, low", compiled[0].ProfileID, compiled[1].ProfileID)
	}
	if !strings.Contains(warnings[0], "bad") {
		t.Errorf("warning %q 未指名被跳过的规则包", warnings[0])
	}
}

// TestCompileSortsFiltersByPriority 注入项之间按 priority 升序（不改内置过滤器顺序）。
func TestCompileSortsFiltersByPriority(t *testing.T) {
	p := profileFixture("sample")
	p.Filters.Extra = []source.FilterRule{
		{Name: "后", Priority: 50, Pattern: "b"},
		{Name: "前", Priority: 5, Pattern: "a"},
	}
	c, err := CompileProfile(p)
	if err != nil {
		t.Fatalf("CompileProfile: %v", err)
	}
	if c.Opts.ExtraFilters[0].Name() != "前" || c.Opts.ExtraFilters[1].Name() != "后" {
		t.Errorf("注入顺序 = %v", []string{c.Opts.ExtraFilters[0].Name(), c.Opts.ExtraFilters[1].Name()})
	}
}
