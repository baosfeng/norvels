package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"norvels/internal/parser"
	"norvels/internal/source"
	"norvels/internal/store"
)

// Compiled 是一份规则包编译后的执行形态：解析选项、切章规则、打标词表与匹配条件。
//
// 编译在导入开始时做一次（865 个文件量级下正则编译是主要成本），之后每个候选只
// 做匹配与解析，不再碰 JSON 与正则编译。
type Compiled struct {
	ProfileID   string
	Name        string
	Priority    int
	Opts        parser.Options
	Rules       []parser.Rule
	Tags        []store.TagRule
	MinChapters int

	matchExts   []string
	matchGlob   string
	matchNameRe *regexp.Regexp
	matchZipRe  *regexp.Regexp
}

// CompileProfile 把一份（已归一化的）规则包编译成执行形态。
//
// 编码名与切章策略名必须在 parser 注册表里：写错名字时解析会静默退回内置策略，
// 用户看到的是「规则包没生效」而不是报错——所以在编译期就拒绝。
func CompileProfile(p source.Profile) (Compiled, error) {
	c := Compiled{
		ProfileID:   strings.TrimSpace(p.ID),
		Name:        p.Name,
		Priority:    p.Priority,
		Opts:        parser.DefaultOptions(),
		MinChapters: p.Limits.MinChapters,
		Tags:        toStoreTags(p.Tags),
	}
	if err := compileMatch(p, &c); err != nil {
		return Compiled{}, err
	}
	if err := compileDecode(p, &c); err != nil {
		return Compiled{}, err
	}
	if err := compileFilters(p, &c); err != nil {
		return Compiled{}, err
	}
	if err := compileMeta(p, &c); err != nil {
		return Compiled{}, err
	}
	return c, compileSplit(p, &c)
}

// CompileProfiles 编译整批规则包：坏包整份跳过并记一条 warning，按 priority 降序
// 排列（同 priority 保持配置顺序）。一条坏规则不该让整次导入失败。
func CompileProfiles(profiles []source.Profile) ([]Compiled, []string) {
	compiled := make([]Compiled, 0, len(profiles))
	warnings := make([]string, 0)
	for _, p := range profiles {
		c, err := CompileProfile(p)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("来源规则包 %q 已跳过：%v", p.ID, err))
			continue
		}
		compiled = append(compiled, c)
	}
	sort.SliceStable(compiled, func(i, j int) bool { return compiled[i].Priority > compiled[j].Priority })
	return compiled, warnings
}

// toStoreTags 把规则包里的标签词表转成 store 的写入口径（深拷贝关键词切片，
// 避免与请求体共享底层数组）。
func toStoreTags(tags []source.TagRule) []store.TagRule {
	out := make([]store.TagRule, 0, len(tags))
	for _, t := range tags {
		out = append(out, store.TagRule{Tag: t.Tag, Keywords: append([]string(nil), t.Keywords...)})
	}
	return out
}

// compileMatch 预编译 match 的条件（正则只编译一次，运行期逐候选比较）。
func compileMatch(p source.Profile, c *Compiled) error {
	c.matchExts = p.Match.Ext
	c.matchGlob = strings.TrimSpace(p.Match.PathGlob)
	var err error
	if c.matchNameRe, err = compileOptional(p.Match.NameRegex); err != nil {
		return fmt.Errorf("importer: 规则包 %q 的 match.nameRegex 非法: %w", p.ID, err)
	}
	if c.matchZipRe, err = compileOptional(p.Match.ZipNameRegex); err != nil {
		return fmt.Errorf("importer: 规则包 %q 的 match.zipNameRegex 非法: %w", p.ID, err)
	}
	return nil
}

// compileOptional 编译可选正则：留空表示不限制（返回 nil）。
func compileOptional(pattern string) (*regexp.Regexp, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, nil
	}
	return regexp.Compile(pattern)
}

// compileDecode 校验强制编码名是否在 parser 注册表里，并写进解析选项。
func compileDecode(p source.Profile, c *Compiled) error {
	enc := strings.ToLower(strings.TrimSpace(p.Decode.ForceEncoding))
	if enc == "" {
		return nil
	}
	if !containsString(codecNames(), enc) {
		return fmt.Errorf("importer: 规则包 %q 的 forceEncoding %q 不在 parser 注册表里（可用：%s）",
			p.ID, enc, strings.Join(codecNames(), ", "))
	}
	c.Opts.ForceEncoding = enc
	return nil
}

// compileFilters 把 filters.disable 直通、把 filters.extra 实例化成 declFilter。
//
// 注入项按 priority 升序排列；它们仍然追加在内置过滤器之后执行（不改 runFilters
// 的既有排序），所以 priority 只在规则包内多条声明式规则之间有意义。
func compileFilters(p source.Profile, c *Compiled) error {
	c.Opts.DisableFilters = append([]string(nil), p.Filters.Disable...)
	for _, r := range p.Filters.Extra {
		f, err := parser.NewDeclFilter(r.Name, r.Priority, r.Pattern, r.Replace)
		if err != nil {
			return fmt.Errorf("importer: 规则包 %q：%w", p.ID, err)
		}
		c.Opts.ExtraFilters = append(c.Opts.ExtraFilters, f)
	}
	sort.SliceStable(c.Opts.ExtraFilters, func(i, j int) bool {
		return c.Opts.ExtraFilters[i].Priority() < c.Opts.ExtraFilters[j].Priority()
	})
	return nil
}

// compileMeta 把 meta 规则实例化成 declMeta（投票权重来自规则包）。
func compileMeta(p source.Profile, c *Compiled) error {
	for _, m := range p.Meta {
		ex, err := parser.NewDeclMeta(m.Name, m.Weight, m.Pattern, m.Title, m.Author, m.Intro)
		if err != nil {
			return fmt.Errorf("importer: 规则包 %q：%w", p.ID, err)
		}
		c.Opts.ExtraMetas = append(c.Opts.ExtraMetas, ex)
	}
	return nil
}

// compileSplit 编译切章层：强制策略名必须在注册表里，规则正则在这里一次编译好。
func compileSplit(p source.Profile, c *Compiled) error {
	sp := strings.ToLower(strings.TrimSpace(p.Split.ForceSplitter))
	if sp != "" {
		if !containsString(splitterNames(), sp) {
			return fmt.Errorf("importer: 规则包 %q 的 forceSplitter %q 不在 parser 注册表里（可用：%s）",
				p.ID, sp, strings.Join(splitterNames(), ", "))
		}
		c.Opts.ForceSplitter = sp
	}
	for i, r := range p.Split.Rules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("importer: 规则包 %q 的第 %d 条切章规则的正则非法: %w", p.ID, i+1, err)
		}
		c.Rules = append(c.Rules, parser.Rule{Name: r.Name, Pattern: re})
	}
	if p.Limits.MaxChapterChars > 0 {
		c.Opts.MaxChapterChars = p.Limits.MaxChapterChars
	}
	return nil
}

// codecNames 返回 parser 注册的编码名（按注册顺序，即候选顺序）。
func codecNames() []string {
	list := parser.Codecs()
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.Name())
	}
	return out
}

// splitterNames 返回 parser 注册的切章策略名。
func splitterNames() []string {
	list := parser.Splitters()
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name())
	}
	return out
}

// containsString 判断字符串切片是否包含目标值。
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
