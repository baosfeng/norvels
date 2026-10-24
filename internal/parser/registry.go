package parser

import "sort"

// 全局注册表。内置策略在包初始化时注册；第三方策略也可以在自己的 init 里注册，
// 或用 Options.Extra* 只对单次解析生效（不污染全局）。
var (
	regCodecs    []Codec
	regFilters   []Filter
	regMetas     []MetaExtractor
	regSplitters []Splitter
)

// init 集中注册全部内置策略，便于一屏总览本包支持哪些格式。
func init() {
	registerBuiltinCodecs()
	registerBuiltinFilters()
	registerBuiltinMetas()
	registerBuiltinSplitters()
}

// RegisterCodec 注册一个编码策略（后注册的同名策略覆盖先注册的）。
func RegisterCodec(c Codec) {
	if c == nil {
		return
	}
	regCodecs = upsert(regCodecs, c, func(x Codec) string { return x.Name() }, c.Name())
}

// RegisterFilter 注册一个过滤器；同名覆盖。
func RegisterFilter(f Filter) {
	if f == nil {
		return
	}
	regFilters = upsert(regFilters, f, func(x Filter) string { return x.Name() }, f.Name())
}

// RegisterMeta 注册一个元数据抽取策略；同名覆盖。
func RegisterMeta(m MetaExtractor) {
	if m == nil {
		return
	}
	regMetas = upsert(regMetas, m, func(x MetaExtractor) string { return x.Name() }, m.Name())
}

// RegisterSplitter 注册一个切章策略；同名覆盖。
func RegisterSplitter(s Splitter) {
	if s == nil {
		return
	}
	regSplitters = upsert(regSplitters, s, func(x Splitter) string { return x.Name() }, s.Name())
}

// Codecs 返回当前注册的编码策略（按注册顺序）。
func Codecs() []Codec { return append([]Codec(nil), regCodecs...) }

// Filters 返回当前注册的过滤器（已按 Priority 升序排列）。
func Filters() []Filter {
	out := append([]Filter(nil), regFilters...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority() < out[j].Priority() })
	return out
}

// Metas 返回当前注册的元数据策略（按 Weight 降序，高可信度先投票）。
func Metas() []MetaExtractor {
	out := append([]MetaExtractor(nil), regMetas...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Weight() > out[j].Weight() })
	return out
}

// Splitters 返回当前注册的切章策略（按注册顺序）。
func Splitters() []Splitter { return append([]Splitter(nil), regSplitters...) }

// upsert 用「同名覆盖」语义写入注册表，保持首次注册的位置不变。
func upsert[T any](list []T, item T, name func(T) string, key string) []T {
	for i, old := range list {
		if name(old) == key {
			list[i] = item
			return list
		}
	}
	return append(list, item)
}
