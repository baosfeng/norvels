package crawl

import (
	"strings"

	"github.com/PuerkitoBio/goquery"

	"norvels/internal/enrich"
	"norvels/internal/ruleexec"
	"norvels/internal/source"
)

// searchFields 是搜索列表要取的五条规则对应的键（Legado 的 ruleSearch 口径）。
var searchFields = []string{"bookList", "name", "author", "intro", "kind", "bookUrl"}

// parseHits 把搜索页 HTML 解析成命中列表。
//
// 解析失败（列表规则为空 / HTML 不合法）不算错误：书源改版是常态，
// 失败只让这个源本次不产命中，熔断计数由 fetch 层负责，这里不重复计。
func parseHits(st *sourceState, body []byte, title string, minSim float64) ([]enrich.Hit, []string, error) {
	rules, degraded := compileSearch(st.profile)
	if rules == nil {
		return nil, degraded, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, degraded, err
	}
	ctx := &ruleexec.Context{HTML: doc, Vars: map[string]string{
		"baseUrl": baseURLOf(st.profile),
		"key":     title,
	}}
	items, err := rules.list.SelectAll(ctx)
	if err != nil {
		return nil, degraded, err
	}
	out := make([]enrich.Hit, 0, len(items))
	for _, item := range items {
		h := enrich.Hit{
			SourceID: st.profile.ID,
			Priority: st.profile.Priority,
			Title:    evalField(rules.name, item),
			Author:   evalField(rules.author, item),
			Intro:    evalField(rules.intro, item),
			Kind:     evalField(rules.kind, item),
			BookURL:  evalField(rules.bookURL, item),
		}
		if h.Title == "" || enrich.TitleSim(title, h.Title) < minSim {
			continue
		}
		out = append(out, h)
	}
	return out, degraded, nil
}

// searchRules 是一个来源编译后的搜索规则集（nil 字段表示该源没配这条规则）。
type searchRules struct {
	list    *ruleexec.Rule
	name    *ruleexec.Rule
	author  *ruleexec.Rule
	intro   *ruleexec.Rule
	kind    *ruleexec.Rule
	bookURL *ruleexec.Rule
}

// compileSearch 编译 ruleSearch 的六条规则，并把 @js 降级信息拼成 "src:字段" 清单。
//
// 编译失败的规则当作「该源没配这条」处理：一条规则写坏不该让整个源不可用，
// 缺失字段留给 enrich.Decide 判定（它会因为拿不到值而不改对应字段）。
func compileSearch(p source.Profile) (*searchRules, []string) {
	degraded := make([]string, 0, len(searchFields))
	r := &searchRules{}
	rules := make([]*ruleexec.Rule, 0, len(searchFields))
	for _, name := range searchFields {
		raw := p.Online.RuleSearch[name]
		if strings.TrimSpace(raw) == "" {
			rules = append(rules, nil)
			continue
		}
		rule, err := ruleexec.Compile(raw)
		if err != nil {
			rules = append(rules, nil)
			continue
		}
		if rule.Degraded {
			degraded = append(degraded, p.ID+":ruleSearch."+name)
		}
		rules = append(rules, rule)
	}
	if rules[0] == nil {
		return nil, degraded
	}
	r.list, r.name, r.author, r.intro, r.kind, r.bookURL =
		rules[0], rules[1], rules[2], rules[3], rules[4], rules[5]
	return r, degraded
}

// evalField 求值一条可选规则：nil 规则或求值出错都返回空串（不猜、不编）。
func evalField(r *ruleexec.Rule, ctx *ruleexec.Context) string {
	if r == nil {
		return ""
	}
	v, err := r.Eval(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}
