// Package enrich 是元数据补全的纯决策层：判定占位作者、比对书名、多源投票、
// 本地词表分类、超长章节切点计算。
//
// 本包刻意只依赖标准库：它同时被 internal/crawl（抓取层）与 cmd/norvels（编排层）使用，
// 一旦依赖 store 或 parser，抓取层就会把解析与存储细节拖进来（见 tests/tools/gatedeps）。
package enrich

import (
	"strings"
	"unicode"
)

// PlaceholderAuthor 是导入链路的兜底作者名，与 store.unknownAuthor / parser 的兜底同值。
// 这里单独定义而不是引用 store：本包是叶子包，不能依赖 store。
const PlaceholderAuthor = "佚名"

// placeholderAuthors 是「不是真人名」的占位集合，按显示顺序排列。
//
// 为什么是白名单而不是正则：占位名的变体是有限且已知的（各站导出工具的默认值），
// 而正则式的「看起来不像人名」黑名单会把真名（两字笔名）误杀。
var placeholderAuthors = []string{
	"佚名", "无名", "未知", "未知作者", "作者未知", "不详", "未知氏",
	"多人", "多人合集", "匿名", "暂无作者", "未署名",
	"unknown", "unknown author", "anonymous", "n/a", "na", "none", "null", "-",
}

// placeholderSet 是 placeholderAuthors 的小写索引，避免每次判定都线性扫一遍。
var placeholderSet = func() map[string]bool {
	m := make(map[string]bool, len(placeholderAuthors))
	for _, n := range placeholderAuthors {
		m[strings.ToLower(n)] = true
	}
	return m
}()

// PlaceholderAuthors 返回占位作者名的副本（稳定顺序），供报告与文档引用。
func PlaceholderAuthors() []string {
	out := make([]string, len(placeholderAuthors))
	copy(out, placeholderAuthors)
	return out
}

// IsPlaceholderAuthor 判定一个作者名是否属于占位值。
//
// 判据是「归一后再查表」：语料里同一个占位名常有全角空格、首尾空白、大小写差异
// （Unknown / unknown），这些不该被当成不同作者。白名单之外再加一条形态判定：
// 纯 ASCII 数字不是人名，见 isAllASCIIDigits。
func IsPlaceholderAuthor(name string) bool {
	n := NormalizeAuthor(name)
	if n == "" {
		return true
	}
	if isAllASCIIDigits(n) {
		return true
	}
	return placeholderSet[strings.ToLower(n)]
}

// isAllASCIIDigits 判定一个已归一的作者名是否「全部由 ASCII 数字组成」。
//
// 为什么按形态判而不进白名单：纯数字不是一个具体名字，而是书源书籍 id 被当作者
// 抽出来的产物，形态判定才能覆盖任意长度。为什么只认 ASCII 数字：含数字的
// 真实笔名（字母+数字、汉字+数字）必须保留，所以只能判「全是数字」；全角数字
// 不在判据内——语料里没有这种形态，宽口径会把「哪些 Unicode 算数字」也变成一条规则。
func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// authorLabelPrefixes 是可机械剥离的作者标签前缀：标签词 + 冒号（全角 / 半角）。
//
// 冒号是必需的判别特征：写作「作者不爱洗澡」这类以「作者」二字开头的真实笔名不带冒号，
// 剥掉就把真名改坏了——所以这里只认带冒号的标签，不做「看着像标签就剥」的猜测。
var authorLabelPrefixes = []string{"作者：", "作者:", "著者：", "著者:"}

// stripAuthorLabelPrefix 剥掉出现在最前面的作者标签前缀（至多一处），并去掉其后残留的空白。
//
// 剥完可能为空（书源只给了一个「作者：」），按调用方既有语义返回空串：
// IsPlaceholderAuthor 会把空名当占位，从而不写库。
func stripAuthorLabelPrefix(s string) string {
	for _, p := range authorLabelPrefixes {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	return s
}

// NormalizeAuthor 归一作者名：剥作者标签前缀、去首尾与内部空白、去书名号/引号、去「著」后缀。
//
// 只做可解释的机械归一，不做拼音/繁简转换——那属于内容判断，不是这里的事。
func NormalizeAuthor(name string) string {
	s := strings.TrimSpace(name)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '《', '》', '〈', '〉', '「', '」', '『', '』', '"', '\'':
			return -1
		}
		if unicode.IsSpace(r) || r == '\u3000' {
			return -1
		}
		return r
	}, s)
	s = stripAuthorLabelPrefix(s)
	if len(s) > len("著") && strings.HasSuffix(s, "著") {
		s = strings.TrimSuffix(s, "著")
	}
	return s
}
