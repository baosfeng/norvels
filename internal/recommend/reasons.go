package recommend

// 推荐理由的两面：给机器看的稳定 code 与给人看的文案。
//
// 为什么要拆：看板的「理由分布」按 extra 分组统计。以前文案本身就是分组键，
// 后端把「因为你常看武侠」改成「根据你的武侠偏好」，历史统计立刻断成两行，
// 而且旧行从此冻结——文案被当成数据维度用了一次。
//
// 现在的分工：
//   - code 进埋点（extra）与看板分组，**改文案不动它**；
//   - 文案只在渲染时产生，前端拿到就展示，不回传。
//
// code 的形态是「类别」或「类别:载荷」：载荷是分类名 / 标签名 / 系列名。
// 载荷只用于看板下钻与文案还原，不参与相等判断以外的逻辑。

import "strings"

// 理由类别（code 的前缀）。
const (
	CodeInterest = "interest" // interest:<分类名>
	CodeTag      = "tag"      // tag:<标签名>
	CodeSeries   = "series"   // series:<系列名>
	CodeCooccur  = "cooccur"  // 读过这本书的人也在看
	CodeHot      = "hot"      // 最近很多人读
	CodeFresh    = "fresh"    // 最近入库
	CodeResume   = "resume"   // 读到一半，接着读
	CodeRandom   = "random"   // 随机发现（尾部探索）
	CodeFallback = "fallback" // 猜你喜欢（无信号时的兜底）

	// 相似书召回的两条「没有载荷」的关系：同作者 / 同分类。
	// 同系列复用 series:<系列名>——两种召回对用户是同一句话（同系列《X》），
	// 分成两个 code 只会在看板上把同一件事拆成两块。
	// 共读邻居也不在这里：它走打分里的 cooccur 分量，文案本来就是「读过这本书的人也在看」。
	CodeSimilarAuthor   = "similar.author"
	CodeSimilarCategory = "similar.category"
)

// Reason 是一条推荐理由：code 稳定、text 可改。
type Reason struct {
	Code string
	Text string
}

// withPayload 拼出「类别:载荷」形态的 code。
func withPayload(kind, payload string) string {
	if payload == "" {
		return kind
	}
	return kind + ":" + payload
}

// payloadOf 取 code 的载荷（没有冒号时返回空串）。
func payloadOf(code string, kind string) string {
	if !strings.HasPrefix(code, kind+":") {
		return ""
	}
	return strings.TrimPrefix(code, kind+":")
}

// InterestReason 拼兴趣类理由（分类/作者兴趣合并成一路，与打分口径一致）。
func InterestReason(c Candidate) Reason {
	for _, cat := range [...]string{c.CategoryL1, c.CategoryL2} {
		if cat != "" {
			return Reason{Code: withPayload(CodeInterest, cat), Text: interestPrefix + cat}
		}
	}
	return Reason{Code: CodeFallback, Text: ReasonDefault}
}

// TagReason 拼标签类理由；没有命中标签时退化为兜底理由。
func TagReason(tag string) Reason {
	if tag == "" {
		return Reason{Code: CodeFallback, Text: ReasonDefault}
	}
	return Reason{Code: withPayload(CodeTag, tag), Text: tagPrefix + tag + tagSuffix}
}

// SeriesReasonFor 拼系列类理由（召回路径与相似书共用同一套文案与 code）。
func SeriesReasonFor(series string) Reason {
	if series == "" {
		return Reason{Code: CodeFallback, Text: ReasonDefault}
	}
	return Reason{Code: withPayload(CodeSeries, series), Text: SeriesReason(series)}
}

// FixedReason 是固定文案类理由（共读/热度/新鲜/续读/随机/兜底）。
func FixedReason(code string) Reason {
	return Reason{Code: code, Text: TextOf(code)}
}

// fixedTexts 是「无载荷 code → 文案」的对照表。
var fixedTexts = map[string]string{
	CodeCooccur:         ReasonCooccur,
	CodeHot:             ReasonHot,
	CodeFresh:           ReasonFresh,
	CodeResume:          ReasonResume,
	CodeRandom:          ReasonRandom,
	CodeFallback:        ReasonDefault,
	CodeSimilarAuthor:   ReasonSameAuthor,
	CodeSimilarCategory: ReasonSameCategory,
}

// payloadTexts 是「带载荷 code 前缀 → 文案模板」的对照表。
//
// 用表而不是 switch：每加一个带载荷的类别就多一个 case，圈复杂度会跟着涨，
// 而这里本质上是「查一张表」。
var payloadTexts = []struct {
	kind string
	text func(string) string
}{
	{CodeInterest, func(v string) string { return interestPrefix + v }},
	{CodeTag, func(v string) string { return tagPrefix + v + tagSuffix }},
	{CodeSeries, SeriesReason},
}

// TextOf 把 code 还原成展示文案（载荷类 code 会重新拼一遍）。
//
// 看板用它给历史数据补 label：三个月前的行只有 code，文案可能已经改过三版。
func TextOf(code string) string {
	if text, ok := fixedTexts[code]; ok {
		return text
	}
	for _, t := range payloadTexts {
		if v := payloadOf(code, t.kind); v != "" {
			return t.text(v)
		}
	}
	return code
}

// CodeFromText 把历史数据里的「文案」映射回 code（认不出返回空串）。
//
// 为什么需要：改这一版之前，extra 里存的是文案本身。rec 库虽然可以整库重建，
// 但重建会丢掉 90 天内的点击分布——能就地归一化就不要让用户丢历史。
// 认不出的一律返回空串，由调用方决定「按原样当未知理由展示」。
func CodeFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if fixed, ok := legacyFixed[text]; ok {
		return fixed
	}
	if strings.HasPrefix(text, interestPrefix) {
		return withPayload(CodeInterest, strings.TrimPrefix(text, interestPrefix))
	}
	if strings.HasPrefix(text, tagPrefix) && strings.HasSuffix(text, tagSuffix) {
		inner := strings.TrimSuffix(strings.TrimPrefix(text, tagPrefix), tagSuffix)
		return withPayload(CodeTag, inner)
	}
	if strings.HasPrefix(text, seriesPrefix) && strings.HasSuffix(text, seriesSuffix) {
		inner := strings.TrimSuffix(strings.TrimPrefix(text, seriesPrefix), seriesSuffix)
		return withPayload(CodeSeries, inner)
	}
	return ""
}

// legacyFixed 是「固定文案 → code」的对照表（只收改动前真实写进 extra 的那几句话）。
var legacyFixed = map[string]string{
	ReasonCooccur:      CodeCooccur,
	ReasonHot:          CodeHot,
	ReasonFresh:        CodeFresh,
	ReasonResume:       CodeResume,
	ReasonRandom:       CodeRandom,
	ReasonDefault:      CodeFallback,
	ReasonSameAuthor:   CodeSimilarAuthor,
	ReasonSameCategory: CodeSimilarCategory,
}
