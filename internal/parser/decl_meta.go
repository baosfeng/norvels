package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// declMeta 是声明式规则包编译出的元数据策略：一条正则 + 捕获组序号。
//
// 匹配范围是「首个章节标题之前的正文头部」（d.preamble()），与内置 preamble
// 策略同一取材口径；文件名相关的规则请在规则包的 match.nameRegex 表达。
// 投票权重来自规则包：Extract 返回的置信分就是它，mergeMeta 用「字段分 × 置信分」
// 与其他策略比较，所以 weight 越大越优先。
type declMeta struct {
	name    string
	weight  int
	pattern *regexp.Regexp
	title   int
	author  int
	intro   int
}

// NewDeclMeta 把一条声明式元数据规则编译成 MetaExtractor。
//
// 捕获组序号 0 表示「不使用该字段」；越界（大于正则的分组数）在编译期拒绝，
// 否则运行期只会静默取空串，用户看到的是「规则包不生效」而不是报错。
func NewDeclMeta(name string, weight int, pattern string, title, author, intro int) (MetaExtractor, error) {
	re, err := regexp.Compile(withLineAnchors(pattern))
	if err != nil {
		return nil, fmt.Errorf("parser: 声明式元数据规则 %q 的正则非法: %w", name, err)
	}
	groups := re.NumSubexp()
	for _, g := range []int{title, author, intro} {
		if g < 0 || g > groups {
			return nil, fmt.Errorf("parser: 声明式元数据规则 %q 的捕获组序号 %d 超出正则的 %d 个分组", name, g, groups)
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "decl-meta"
	}
	return &declMeta{name: name, weight: weight, pattern: re, title: title, author: author, intro: intro}, nil
}

// Name 返回规则包里的展示名。
func (m *declMeta) Name() string { return m.name }

// Weight 返回规则包里的权重（越大越可信）。
func (m *declMeta) Weight() int { return m.weight }

// Extract 在正文头部匹配一次，按捕获组序号给字段打分；没有任何字段命中时放弃。
func (m *declMeta) Extract(d *Doc) (Meta, int) {
	sub := m.pattern.FindStringSubmatch(d.preamble())
	if sub == nil {
		return Meta{}, 0
	}
	var out Meta
	if v := captureAt(sub, m.title); v != "" {
		out.Title, out.TitleScore = v, 100
	}
	if v := captureAt(sub, m.author); v != "" {
		out.Author, out.AuthorScore = v, 100
	}
	if v := captureAt(sub, m.intro); v != "" {
		out.Intro, out.IntroScore = v, 100
	}
	if out.Title == "" && out.Author == "" && out.Intro == "" {
		return Meta{}, 0
	}
	return out, m.weight
}

// captureAt 取一个捕获组并去空白；序号 0 / 越界 / 未参与匹配都返回空串。
func captureAt(sub []string, idx int) string {
	if idx <= 0 || idx >= len(sub) {
		return ""
	}
	return strings.TrimSpace(sub[idx])
}
