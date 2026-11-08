package enrich

import "strings"

// categoryRule 是一条本地分类词表：命中任一关键词即归入 l1/l2。
type categoryRule struct {
	l1 string
	l2 string
	kw []string
}

// categoryRules 是分类词表，顺序即匹配优先级（越靠前越先判）。
//
// 为什么用本地词表而不是全靠书源：18 万本里绝大多数书在任何书源上都搜不到，
// 词表能在离线状态下把「书名里就写着玄幻」的那批先归好，书源只处理剩下的。
// 关键词只用题材词，不含任何真实书名/作者（隐私红线）。
var categoryRules = []categoryRule{
	{l1: "玄幻", l2: "东方玄幻", kw: []string{"斗气", "武魂", "异界", "玄幻", "神帝", "武帝", "万古", "吞噬"}},
	{l1: "仙侠", l2: "修真", kw: []string{"修仙", "修真", "仙侠", "仙尊", "仙帝", "飞升", "渡劫", "灵根", "剑仙", "洪荒"}},
	{l1: "都市", l2: "都市异能", kw: []string{"都市", "校花", "兵王", "总裁", "重生之都市", "保镖", "高手在", "神豪"}},
	{l1: "历史", l2: "架空历史", kw: []string{"大唐", "大明", "三国", "抗战", "铁血", "军阀", "锦衣卫", "回到明朝", "大清", "宋朝", "秦汉"}},
	{l1: "科幻", l2: "末世", kw: []string{"星际", "机甲", "末世", "末日", "银河", "太空", "赛博", "虫族", "未来世界", "废土"}},
	{l1: "游戏", l2: "虚拟网游", kw: []string{"网游", "游戏", "电竞", "副本", "全服", "公会", "玩家"}},
	{l1: "武侠", l2: "传统武侠", kw: []string{"武侠", "江湖", "侠客", "门派", "剑客"}},
	{l1: "悬疑灵异", l2: "灵异", kw: []string{"悬疑", "灵异", "盗墓", "捉鬼", "诡事", "推理", "惊悚"}},
	{l1: "军事", l2: "军旅", kw: []string{"军事", "战争", "特种兵", "军旅", "雇佣兵", "战场"}},
	{l1: "体育", l2: "竞技", kw: []string{"足球", "篮球", "体育", "竞技"}},
	{l1: "奇幻", l2: "西幻", kw: []string{"魔法", "精灵", "巫师", "骑士", "巨龙", "西幻"}},
	{l1: "轻小说", l2: "轻小说", kw: []string{"轻小说", "同人", "二次元"}},
}

// Classify 用本地词表按「书名 + 简介前 200 字」判分类。
//
// 只看前 200 字简介：简介里的分类线索几乎都在开头（"【玄幻】……"），
// 通读全文既慢又容易被正文里的无关词带偏。ok=false 表示词表没命中，交给书源或保持原值。
func Classify(title, intro string) (l1, l2 string, ok bool) {
	if hit, found := matchRules(title); found {
		return hit.l1, hit.l2, true
	}
	if hit, found := matchRules(headRunes(intro, 200)); found {
		return hit.l1, hit.l2, true
	}
	return "", "", false
}

// matchRules 在给定文本上按词表顺序找第一条命中的规则。
func matchRules(text string) (categoryRule, bool) {
	if strings.TrimSpace(text) == "" {
		return categoryRule{}, false
	}
	for _, r := range categoryRules {
		for _, k := range r.kw {
			if strings.Contains(text, k) {
				return r, true
			}
		}
	}
	return categoryRule{}, false
}

// headRunes 取前 n 个字符（按 rune，避免切坏汉字）；不足 n 时原样返回。
func headRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

// SplitCategory 把 "L1/L2" 形式的分类串拆成两段；没有 "/" 时 L2 为空。
//
// 书源给的 kind 常见形态是 "玄幻,东方玄幻" 或 "玄幻/东方玄幻"，
// 这里只认 "/"，其余分隔符由调用方（crawl）在取字段时先用统一管道归一。
func SplitCategory(v string) (l1, l2 string) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, "/"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// JoinCategory 把两级分类拼成 SplitCategory 的逆形式；两级都空时返回空串。
func JoinCategory(l1, l2 string) string {
	l1, l2 = strings.TrimSpace(l1), strings.TrimSpace(l2)
	if l2 == "" {
		return l1
	}
	return l1 + "/" + l2
}
