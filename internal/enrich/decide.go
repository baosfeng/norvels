package enrich

import (
	"sort"
	"strings"
)

// Subject 是库里一本书的现状（只读字段，用于判断「要不要改」）。
type Subject struct {
	Title      string
	Author     string
	Intro      string
	CategoryL1 string
	CategoryL2 string
}

// Hit 是一条书源命中：某个源对某本书给出的元数据。
type Hit struct {
	SourceID string
	Priority int // 大者优先（同优先级按 SourceID 升序，保证结果稳定）
	Title    string
	Author   string
	Intro    string
	Kind     string // 源给的分类串，常见 "玄幻/东方玄幻" 或单独一级
	BookURL  string
}

// FieldValue 是一次字段决策的结果。
type FieldValue struct {
	Value      string
	Confidence float64  // 0~1：命中票数占比
	Sources    []string // 参与投票的 sourceId（稳定排序）
	Changed    bool     // false = 与现值相同，不必写库
}

// Decision 是一本书的建议改动；nil 表示该字段不写。
type Decision struct {
	Author   *FieldValue
	Intro    *FieldValue
	Category *FieldValue
	// Skipped 是跳过原因（稳定顺序），供报告统计，例如 ["author:low_confidence"]。
	Skipped []string
}

// 决策字段名（与 store.EnrichField* 同值；enrich 是叶子包，不反向依赖 store）。
const (
	FieldAuthor   = "author"
	FieldIntro    = "intro"
	FieldCategory = "category"
)

// Options 是决策阈值；零值由 withDefaults 补齐。
type Options struct {
	MinTitleSim   float64 // 命中书名与库内书名的相似度下限
	MinIntroRunes int     // 简介最少字符数
	MinConfidence float64 // 单字段置信度下限
	// Fields 限定只决策哪些字段（取值见 Field*）；nil/空表示三个字段全判。
	// CLI 的 --fields 通过它进入决策层：未选中的字段既不产生变更，也不产生跳过原因。
	Fields map[string]bool
}

// DefaultOptions 返回推荐阈值（与 docs/specs/2026-11-08-metadata-enrich-design.md §6 一致）。
func DefaultOptions() Options {
	return Options{MinTitleSim: 0.86, MinIntroRunes: 30, MinConfidence: 0.6}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.MinTitleSim <= 0 {
		o.MinTitleSim = d.MinTitleSim
	}
	if o.MinIntroRunes <= 0 {
		o.MinIntroRunes = d.MinIntroRunes
	}
	if o.MinConfidence <= 0 {
		o.MinConfidence = d.MinConfidence
	}
	return o
}

// wants 报告字段是否在本次决策范围内（空集合 = 全部）。
func (o Options) wants(field string) bool {
	return len(o.Fields) == 0 || o.Fields[field]
}

// Decide 把多源命中汇总成一本书的建议改动。
//
// 三条取舍：
//  1. 相似度不够的命中直接丢弃（同名不同书是错误元数据的最大来源）；
//  2. 每个字段独立投票，不清空已有值——「有值就不动」比「猜一个更好的」安全；
//  3. 置信度不足的字段不写，只在 Skipped 里留痕，报告能解释「为什么这本没变」。
func Decide(s Subject, hits []Hit, opt Options) Decision {
	o := opt.withDefaults()
	d := Decision{Skipped: []string{}}
	useful := filterHits(s.Title, hits, o)
	if len(useful) == 0 {
		// 没有书源命中不等于「无事可做」：分类可以完全靠本地词表离线判定
		// （生产 10 万本未分类里，绝大多数书的名字里就带着题材词）。
		// 作者与简介仍然只能靠书源，这里只放分类这一路。
		d.Skipped = append(d.Skipped, "no_hit")
		if o.wants(FieldCategory) {
			d.Category = decideCategory(s, nil, o, &d)
		}
		return d
	}
	if o.wants(FieldAuthor) {
		d.Author = decideAuthor(s, useful, o, &d)
	}
	if o.wants(FieldIntro) {
		d.Intro = decideIntro(s, useful, o, &d)
	}
	if o.wants(FieldCategory) {
		d.Category = decideCategory(s, useful, o, &d)
	}
	return d
}

// filterHits 丢掉书名不相似的命中，并按源优先级排序（大者在前、同优先级按 id）。
func filterHits(title string, hits []Hit, o Options) []Hit {
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		if h.SourceID == "" {
			continue
		}
		if TitleSim(title, h.Title) < o.MinTitleSim {
			continue
		}
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

// decideAuthor 只在「现值是占位名」时改：真实作者一旦有值就不覆盖。
func decideAuthor(s Subject, hits []Hit, o Options, d *Decision) *FieldValue {
	if !IsPlaceholderAuthor(s.Author) {
		d.Skipped = append(d.Skipped, "author:already_present")
		return nil
	}
	v, ok := vote(hits, func(h Hit) string {
		if IsPlaceholderAuthor(h.Author) {
			return ""
		}
		return NormalizeAuthor(h.Author)
	}, o)
	if !ok {
		d.Skipped = append(d.Skipped, "author:placeholder_only")
		return nil
	}
	if NormalizeAuthor(s.Author) == v.Value {
		v.Changed = false
		return v
	}
	v.Changed = true
	return v
}

// decideIntro 取达到最小长度的最长简介；已有简介达到新值 80% 长度时不覆盖。
//
// 为什么是「最长」而不是「票数最多」：简介没有标准答案，站点之间的差异主要是截断，
// 最长的那个通常最完整。80% 的容忍度是为了不让「几乎一样长」的替换白白写库。
func decideIntro(s Subject, hits []Hit, o Options, d *Decision) *FieldValue {
	cur := strings.TrimSpace(s.Intro)
	best, bestLen := "", 0
	sources := make([]string, 0, len(hits))
	for _, h := range hits {
		text := strings.TrimSpace(h.Intro)
		n := len([]rune(text))
		if n < o.MinIntroRunes {
			continue
		}
		if n > bestLen {
			best, bestLen = text, n
			sources = sources[:0]
		}
		if text == best {
			sources = append(sources, h.SourceID)
		}
	}
	if best == "" {
		d.Skipped = append(d.Skipped, "intro:too_short")
		return nil
	}
	if curRunes := len([]rune(cur)); curRunes > 0 && curRunes*10 >= bestLen*8 {
		d.Skipped = append(d.Skipped, "intro:already_present")
		return nil
	}
	return &FieldValue{
		Value: best, Confidence: confidence(len(sources), len(hits)),
		Sources: sortedSources(sources), Changed: cur != best,
	}
}

// decideCategory 先走本地词表（离线、零成本），词表没命中才用书源给的 kind 投票。
//
// 已有分类（不是「未分类」）时不覆盖：分类是用户能自己改的，外部结论不该盖掉它。
func decideCategory(s Subject, hits []Hit, o Options, d *Decision) *FieldValue {
	if l1 := strings.TrimSpace(s.CategoryL1); l1 != "" && l1 != Unclassified {
		d.Skipped = append(d.Skipped, "category:already_present")
		return nil
	}
	if l1, l2, ok := Classify(s.Title, s.Intro); ok {
		return categoryValue(s, JoinCategory(l1, l2), []string{"local:keyword"}, 1)
	}
	v, ok := vote(hits, func(h Hit) string {
		l1, _ := SplitCategory(h.Kind)
		if strings.TrimSpace(l1) == "" {
			return ""
		}
		return strings.TrimSpace(h.Kind)
	}, o)
	if !ok {
		d.Skipped = append(d.Skipped, "category:no_hit")
		return nil
	}
	return categoryValue(s, v.Value, v.Sources, v.Confidence)
}

// categoryValue 组装分类决策；与现值相同则 Changed=false。
func categoryValue(s Subject, value string, sources []string, conf float64) *FieldValue {
	return &FieldValue{
		Value: value, Confidence: conf, Sources: sources,
		Changed: JoinCategory(s.CategoryL1, s.CategoryL2) != value,
	}
}

// Unclassified 是存量书回填用的中性分类名（与 store.unclassifiedCategory 同值）。
const Unclassified = "未分类"

// vote 按「源优先级加权」对 extract 出来的值计票，返回得票最高且过阈值的一项。
//
// 权重用 1 + priority/100：优先级只影响同票时的排序倾向，不让一个高优先级源
// 独自压过其他所有源（单源错名的概率并不比多源一致低）。
func vote(hits []Hit, extract func(Hit) string, o Options) (*FieldValue, bool) {
	weight := make(map[string]float64, len(hits))
	sources := make(map[string][]string, len(hits))
	total, best, bestScore := 0.0, "", 0.0
	for _, h := range hits {
		total++
		v := extract(h)
		if v == "" {
			continue
		}
		weight[v] += 1 + float64(h.Priority)/100
		sources[v] = append(sources[v], h.SourceID)
		if weight[v] > bestScore || (weight[v] == bestScore && v < best) {
			best, bestScore = v, weight[v]
		}
	}
	if best == "" || total == 0 {
		return nil, false
	}
	conf := confidence(len(sources[best]), len(hits))
	if conf < o.MinConfidence {
		return nil, false
	}
	return &FieldValue{Value: best, Confidence: conf, Sources: sortedSources(sources[best])}, true
}

// confidence 把「支持票数 / 总命中数」写成 0~1。
func confidence(support, total int) float64 {
	if total <= 0 {
		return 0
	}
	if support > total {
		support = total
	}
	return float64(support) / float64(total)
}

// sortedSources 去重并排序，保证报告与落库的 sources 稳定可比。
func sortedSources(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
