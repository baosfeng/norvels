package importer

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"norvels/internal/store"
)

// profileSelector 把编译结果包装成「按候选选一份规则包」的查询入口。
type profileSelector struct{ compiled []Compiled }

// selectProfile 按候选选一份已编译规则包；nil 表示走内置策略。
func (o Options) selectProfile(c candidate) *Compiled {
	if len(o.compiled) == 0 {
		return nil
	}
	return profileSelector{compiled: o.compiled}.pick(o.ProfileID, c)
}

// pick 按「显式指定 > match 命中且 priority 最高 > 内置」选一份规则包。
//
// compiled 已按 priority 降序排列，第一个全部条件命中的即胜；同一文件只应用
// 一份规则包（不做多包叠加）。显式指定了不存在的 id 时返回 nil（api 层已回 400，
// 直接调用方由 validateExplicitProfile 兜底）。
func (s profileSelector) pick(explicit string, c candidate) *Compiled {
	if explicit != "" {
		for i := range s.compiled {
			if s.compiled[i].ProfileID == explicit {
				return &s.compiled[i]
			}
		}
		return nil
	}
	for i := range s.compiled {
		if matchCandidate(&s.compiled[i], c) {
			return &s.compiled[i]
		}
	}
	return nil
}

// matchCandidate 判断一份规则包是否认领这个候选：ext / pathGlob / nameRegex /
// zipNameRegex 全「与」；留空的条件不参与判断。
func matchCandidate(pc *Compiled, c candidate) bool {
	if !matchExt(pc.matchExts, candidateFileName(c)) {
		return false
	}
	if pc.matchGlob != "" && !globMatch(pc.matchGlob, c.source) {
		return false
	}
	if pc.matchNameRe != nil && !pc.matchNameRe.MatchString(candidateFileName(c)) {
		return false
	}
	if pc.matchZipRe != nil && !pc.matchZipRe.MatchString(zipPackageName(c)) {
		return false
	}
	return true
}

// candidateFileName 取条目的文件名：zip 条目是「包名.zip:包内路径」里的包内文件名。
func candidateFileName(c candidate) string {
	name := c.name
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	return filepath.Base(name)
}

// zipPackageName 取 zip 条目的包名（含扩展名）；不是 zip 条目时返回空串。
func zipPackageName(c candidate) string {
	i := strings.LastIndex(c.name, ":")
	if i < 0 {
		return ""
	}
	return filepath.Base(c.name[:i])
}

// matchExt 判断扩展名是否在允许列表；列表为空表示不限制。
func matchExt(exts []string, name string) bool {
	if len(exts) == 0 {
		return true
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	for _, want := range exts {
		if strings.ToLower(strings.TrimPrefix(want, ".")) == ext {
			return true
		}
	}
	return false
}

// globMatch 支持「前缀/**」的递归通配；其余按 path.Match 精确匹配。
//
// path.Match 的 ** 不等价于跨目录，所以 /books/示例/** 这种写法在这里按
// 「/books/示例/ 开头的任意路径」理解，与设计稿示例口径一致。
func globMatch(pattern, target string) bool {
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(target, strings.TrimSuffix(pattern, "**"))
	}
	ok, err := path.Match(pattern, target)
	return err == nil && ok
}

// ErrProfileNotFound 表示显式指定的规则包 id 未配置（api 映射 400）。
var ErrProfileNotFound = errors.New("importer: 指定的来源规则包未配置")

// prepareProfiles 编译规则包并把结果挂到 Options 上（入口调用一次）；返回的
// warning 由调用方展示或记日志。没有配置规则包时原样返回。
func (o Options) prepareProfiles() (Options, []string) {
	if len(o.Profiles) == 0 {
		return o, nil
	}
	compiled, warnings := CompileProfiles(o.Profiles)
	o.compiled = compiled
	return o, warnings
}

// validateExplicitProfile 确认显式指定的规则包已配置且编译通过。
func (o Options) validateExplicitProfile() error {
	if o.ProfileID == "" {
		return nil
	}
	for i := range o.compiled {
		if o.compiled[i].ProfileID == o.ProfileID {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrProfileNotFound, o.ProfileID)
}

// tagsFor 返回该候选生效的打标词表：规则包的 tags 在前，全局词表追加在后。
func (o Options) tagsFor(pc *Compiled) []store.TagRule {
	if pc == nil || len(pc.Tags) == 0 {
		return o.TagRules
	}
	out := make([]store.TagRule, 0, len(pc.Tags)+len(o.TagRules))
	out = append(out, pc.Tags...)
	return append(out, o.TagRules...)
}

// profileIDOf 取编译结果的 id（nil 返回空串，表示走内置策略）。
func profileIDOf(pc *Compiled) string {
	if pc == nil {
		return ""
	}
	return pc.ProfileID
}

// minChaptersOf 取规则包覆盖的上架门槛（nil 或 0 表示用全局值）。
func minChaptersOf(pc *Compiled) int {
	if pc == nil {
		return 0
	}
	return pc.MinChapters
}
