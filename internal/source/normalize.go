package source

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Normalize 归一化整批规则包：去首尾空白、扩展名小写去点、缺省 schemaVersion/kind
// 补齐、规则名留空补「规则N」、标签关键词去重。
//
// 只在写入边界调用一次（api 的 PUT / validate），落库的是归一化后的形态，
// GET 原样返回同一份 JSON，因此「保存后读回」是稳定的。归一化不改变用户可见语义：
// 不拒绝、不静默删除内容，只做等价改写。
func Normalize(profiles []Profile) []Profile {
	out := make([]Profile, len(profiles))
	for i := range profiles {
		p := profiles[i]
		p.normalize()
		out[i] = p
	}
	return out
}

// normalize 是单份规则包的归一化入口；按层拆分，避免一个函数铺满整页。
func (p *Profile) normalize() {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	if p.SchemaVersion == 0 {
		p.SchemaVersion = SchemaVersion
	}
	if p.Kind == "" {
		p.Kind = KindLocal
	}
	p.Match.Ext = normalizeExts(p.Match.Ext)
	p.Match.PathGlob = strings.TrimSpace(p.Match.PathGlob)
	p.Match.NameRegex = strings.TrimSpace(p.Match.NameRegex)
	p.Match.ZipNameRegex = strings.TrimSpace(p.Match.ZipNameRegex)
	p.Format = normalizeExts(p.Format)
	p.Decode.ForceEncoding = strings.ToLower(strings.TrimSpace(p.Decode.ForceEncoding))
	p.Filters.normalize()
	p.Split.normalize()
	p.normalizeMeta()
	p.Tags = normalizeTags(p.Tags)
	if p.Online != nil {
		p.Online.normalize()
	}
}

// normalizeExts 归一化扩展名/格式列表：去点、小写、去空、去重（保留首次出现顺序）。
func normalizeExts(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "."))
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		out = append(out, ext)
	}
	return out
}

// normalize 归一化过滤层：名字留空补 extra-N（前端列表不留空标题）。
func (f *Filters) normalize() {
	for i := range f.Disable {
		f.Disable[i] = strings.TrimSpace(f.Disable[i])
	}
	for i := range f.Extra {
		r := &f.Extra[i]
		r.Name = ruleName(r.Name, i, "extra-")
		r.Pattern = strings.TrimSpace(r.Pattern)
		r.Replace = strings.TrimSpace(r.Replace)
	}
}

// normalize 归一化切章层：策略名小写，规则名留空补「规则N」（与章节规则同口径）。
func (s *Split) normalize() {
	s.ForceSplitter = strings.ToLower(strings.TrimSpace(s.ForceSplitter))
	for i := range s.Rules {
		r := &s.Rules[i]
		r.Name = chapterRuleName(r.Name, i)
		r.Pattern = strings.TrimSpace(r.Pattern)
	}
}

// normalizeMeta 归一化元数据规则：名字留空补 meta-N，超长截断（名字只用于展示）。
func (p *Profile) normalizeMeta() {
	for i := range p.Meta {
		m := &p.Meta[i]
		m.Name = ruleName(m.Name, i, "meta-")
		m.Pattern = strings.TrimSpace(m.Pattern)
	}
}

// normalizeTags 归一化标签词表：标签与关键词去空白、去空项、去重（保留首次出现顺序）。
func normalizeTags(in []TagRule) []TagRule {
	if len(in) == 0 {
		return in
	}
	out := make([]TagRule, 0, len(in))
	for _, raw := range in {
		t := TagRule{Tag: strings.TrimSpace(raw.Tag)}
		seen := make(map[string]bool, len(raw.Keywords))
		t.Keywords = make([]string, 0, len(raw.Keywords))
		for _, kw := range raw.Keywords {
			k := strings.TrimSpace(kw)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			t.Keywords = append(t.Keywords, k)
		}
		out = append(out, t)
	}
	return out
}

// normalize 归一化在线段：URL/分组去空白，规则块键值去空白并丢弃空键。
func (o *Online) normalize() {
	o.BookSourceURL = strings.TrimSpace(o.BookSourceURL)
	o.Group = strings.TrimSpace(o.Group)
	o.SearchURL = strings.TrimSpace(o.SearchURL)
	o.ExploreURL = strings.TrimSpace(o.ExploreURL)
	o.BookURLPattern = strings.TrimSpace(o.BookURLPattern)
	o.RuleSearch = normalizeRuleBlock(o.RuleSearch)
	o.RuleExplore = normalizeRuleBlock(o.RuleExplore)
	o.RuleBookInfo = normalizeRuleBlock(o.RuleBookInfo)
	o.RuleToc = normalizeRuleBlock(o.RuleToc)
	o.RuleContent = normalizeRuleBlock(o.RuleContent)
	o.Header = normalizeRuleBlock(o.Header)
	o.UnsupportedFields = normalizeNames(o.UnsupportedFields)
}

// normalizeNames 归一化字段名列表：去空白、去空项、去重（保留首次出现顺序）。
func normalizeNames(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// normalizeRuleBlock 归一化一个在线规则块；空块原样返回（nil 保持 nil）。
func normalizeRuleBlock(in map[string]string) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}

// ruleName 归一化规则名：留空补「前缀N」，超长截断到 MaxNameRunes。
func ruleName(raw string, pos int, prefix string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = fmt.Sprintf("%s%d", prefix, pos+1)
	}
	return truncateRunes(name, MaxNameRunes)
}

// chapterRuleName 与 api 的章节规则命名同口径：留空补「规则N」，超长截断。
func chapterRuleName(raw string, pos int) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = fmt.Sprintf("规则%d", pos+1)
	}
	return truncateRunes(name, MaxNameRunes)
}

// truncateRunes 按字符（而非字节）截断，避免切坏多字节汉字。
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
