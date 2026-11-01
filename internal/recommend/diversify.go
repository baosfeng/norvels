package recommend

// 重排的多样性上限（设计 §3）：同作者 ≤2 本、同大分类 ≤4 本。
//
// 为什么作者比分类收得更紧：一个作者写出来的书风格高度同质，
// 连着推三本同一作者，用户看到的几乎是同一本书的三个封面。
const (
	defaultMaxPerAuthor   = 2
	defaultMaxPerCategory = 4
)

// diversityCaps 是多样性配额的计数器（按作者 / 一级分类各记一份）。
type diversityCaps struct {
	maxPerAuthor   int
	maxPerCategory int
	authors        map[int64]int
	categories     map[string]int
}

// newCaps 建配额计数器；≤0 的上限取默认值（调用方不关心参数时传 0 即可）。
func newCaps(limit, maxPerAuthor, maxPerCategory int) *diversityCaps {
	if maxPerAuthor <= 0 {
		maxPerAuthor = defaultMaxPerAuthor
	}
	if maxPerCategory <= 0 {
		maxPerCategory = defaultMaxPerCategory
	}
	return &diversityCaps{
		maxPerAuthor:   maxPerAuthor,
		maxPerCategory: maxPerCategory,
		authors:        make(map[int64]int, limit),
		categories:     make(map[string]int, limit),
	}
}

// accept 判断是否还能收下这本书；能收就顺手占一个配额。
//
// 作者 id 为 0 / 分类为空时不占配额：这类候选通常来自「补位」（数据缺失），
// 给它们单独建一个全 0 的桶会把所有补位书互相挤掉。
func (c *diversityCaps) accept(it Scored) bool {
	if it.AuthorID > 0 && c.authors[it.AuthorID] >= c.maxPerAuthor {
		return false
	}
	if it.CategoryL1 != "" && c.categories[it.CategoryL1] >= c.maxPerCategory {
		return false
	}
	if it.AuthorID > 0 {
		c.authors[it.AuthorID]++
	}
	if it.CategoryL1 != "" {
		c.categories[it.CategoryL1]++
	}
	return true
}

// Diversify 贪心重排：按给定顺序挑，超配额的跳过，直到凑满 limit。
//
// items 必须已按分数降序（Rank 的输出顺序）。这里不重排、也不为凑满配额而插队：
// 多样性是「过滤」不是「加权」，把低分项提前会让推荐分的解释彻底失真。
func Diversify(items []Scored, limit, maxPerAuthor, maxPerCategory int) []Scored {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	caps := newCaps(limit, maxPerAuthor, maxPerCategory)
	out := make([]Scored, 0, min(limit, len(items)))
	for _, it := range items {
		if !caps.accept(it) {
			continue
		}
		out = append(out, it)
		if len(out) >= limit {
			break
		}
	}
	return out
}
