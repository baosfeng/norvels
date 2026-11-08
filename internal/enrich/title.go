package enrich

import (
	"strings"
	"unicode"
)

// NormalizeTitle 把书名归一成可比较形式：去书名号与括号装饰、去标点空白、全角转半角、英文小写。
//
// 为什么这么狠：真实语料里同一本书在库内与书源上的写法差异几乎全在这些装饰上
// （《X》/ X(全本) / 【X】完结 / ｘ），标题主干是稳定的。
func NormalizeTitle(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		r = toHalfWidth(r)
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// toHalfWidth 把全角 ASCII 与全角空格折成半角，其余字符原样返回。
func toHalfWidth(r rune) rune {
	switch {
	case r == '\u3000':
		return ' '
	case r >= '\uFF01' && r <= '\uFF5E':
		return r - 0xFEE0
	}
	return r
}

// TitleSim 返回两个书名的相似度 0~1：归一后按「1 - 编辑距离/较长长度」计算。
//
// 归一后为空的一侧视为不相似（0）：空标题参与投票会把任意命中都算成匹配。
func TitleSim(a, b string) float64 {
	x, y := NormalizeTitle(a), NormalizeTitle(b)
	if x == "" || y == "" {
		return 0
	}
	if x == y {
		return 1
	}
	longer := len(x)
	if len(y) > longer {
		longer = len(y)
	}
	return 1 - float64(editDistance(x, y))/float64(longer)
}

// editDistance 是 Levenshtein 距离（按字节比较即可：两边都已归一为 UTF-8，
// 中文按字节计算相当于按「1/3 个汉字」计权，对相似度的排序结论没有影响，
// 而且避免了 []rune 分配的额外内存）。
func editDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = minInt(prev[j]+1, minInt(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
