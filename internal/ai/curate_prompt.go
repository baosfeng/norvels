package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// curateSystemPrompt 是上架判定的提示词。
//
// 三条纪律写进提示词而不是靠调用方拼：模型最容易跑偏的方向恰好是三条常识——
// 去评文笔、去比较经典、去复述统计数字。这里把「只看三件事」写死，
// 让结论能落到可核对的证据上（理由必须引用看到的数字或片段）。
const curateSystemPrompt = `你是中文小说私人书库的入库审核员。
用户会给你一本书的统计信息、规则已算出的质量指标、以及开头几段正文。请判断：这本书值不值得进他的书库。

只看三件事，不要评价文笔好坏、不要比较是否经典、不要复述统计数字：
1. 能不能读：正文是否完整可读——不是乱码、不是广告或目录页、不是残缺的网页抓取；
2. 是不是残卷：首章像简介、末章在一句话中间截断、章节数相对字数明显偏少；
3. 是不是拼凑或重复：多本书被塞进一个文件、正文与书名对不上、同一段内容反复出现。

只输出一个 JSON 对象，不要代码块，不要任何解释文字：
{"decision":"import|skip|review","score":0,"reasons":["..."],"category":"","tags":[]}
- decision：import 值得入库；skip 不值得（理由要说清是哪一条踩中了）；review 拿不准，交人看；
- score：0 到 100 的整数，表示这本书进书库的价值（不是文笔分，也不是质量分）；
- reasons：1 到 3 条简短依据（中文，每条不超过 30 字），必须引用你看到的证据，
  例如「末章正文在一句话中间截断」「开头三段都是网址广告」；
- category：只能从「已有分类」里挑一个原样写回，都不合适就填空字符串；
- tags：最多 5 个中文标签（题材/流派），没有就给空数组。`

// renderCard 把书卡渲染成送审正文（模型看到的就是这一段）。
func renderCard(c BookCard) string {
	var b strings.Builder
	writeCardHeader(&b, c)
	writeCardMetrics(&b, c)
	writeCardCategories(&b, c)
	writeCardSamples(&b, c)
	return strings.TrimRight(b.String(), "\n")
}

// writeCardHeader 写身份与规模字段。
func writeCardHeader(b *strings.Builder, c BookCard) {
	fmt.Fprintf(b, "书名：%s\n", orNone(c.Title))
	fmt.Fprintf(b, "作者：%s\n", orNone(c.Author))
	fmt.Fprintf(b, "来源文件：%s\n", orNone(c.Source))
	fmt.Fprintf(b, "格式：%s", orNone(c.Format))
	if c.Encoding != "" {
		fmt.Fprintf(b, "／编码：%s", c.Encoding)
	}
	fmt.Fprintf(b, "／大小：%s\n", humanBytes(c.Bytes))
	avg := 0
	if c.Chapters > 0 {
		avg = c.Words / c.Chapters
	}
	fmt.Fprintf(b, "字数：%d／章节数：%d／平均章长：%d 字\n", c.Words, c.Chapters, avg)
}

// writeCardMetrics 写规则侧质量指标。
func writeCardMetrics(b *strings.Builder, c BookCard) {
	fmt.Fprintf(b, "质量指标：噪声率 %.1f%%／乱码率 %.1f%%／结构分 %.2f／书内重复率 %.1f%%\n",
		c.NoiseRatio*100, c.GarbledRatio*100, c.StructureScore, c.DuplicateRatio*100)
}

// writeCardCategories 写可选分类（模型只能从中挑一个）。
func writeCardCategories(b *strings.Builder, c BookCard) {
	if len(c.Categories) == 0 {
		fmt.Fprintf(b, "已有分类：（空）\n")
		return
	}
	cats := append([]string(nil), c.Categories...)
	sort.Strings(cats)
	if len(cats) > maxCardCategories {
		cats = cats[:maxCardCategories]
	}
	fmt.Fprintf(b, "已有分类：%s\n", strings.Join(cats, "、"))
}

// writeCardSamples 写样章正文（每段截断，最多 maxCardSamples 段）。
func writeCardSamples(b *strings.Builder, c BookCard) {
	if len(c.Samples) == 0 {
		return
	}
	fmt.Fprintf(b, "开头正文片段：\n")
	shown := 0
	for _, s := range c.Samples {
		text := truncateRunes(s, maxSampleRunes)
		if text == "" {
			continue
		}
		shown++
		fmt.Fprintf(b, "[%d] %s\n", shown, text)
		if shown == maxCardSamples {
			break
		}
	}
}

// orNone 把空串渲染成「未知」，避免模型把空字段当成「这书没名字」而误判为垃圾。
func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "未知"
	}
	return s
}

// humanBytes 把字节数渲染成 MB/KB（只用于给模型一个量级感）。
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// curationPayload 是模型该吐的 JSON 形态。
type curationPayload struct {
	Decision string   `json:"decision"`
	Score    int      `json:"score"`
	Reasons  []string `json:"reasons"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

// parseCuration 解析模型给出的判定；allowed 是库内已有分类（空表示不限）。
func parseCuration(content string, allowed []string) (Curation, error) {
	raw := stripFence(content)
	if raw == "" {
		return Curation{}, fmt.Errorf("%w：响应为空", ErrBadResponse)
	}
	var p curationPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Curation{}, fmt.Errorf("%w：%v", ErrBadResponse, err)
	}
	return p.normalize(allowed)
}

// normalize 校验并归一化字段：decision 必须是三态之一，score 落在 0..100。
//
// score 越界直接报错而不是截断：那是提示词被改坏或模型跑偏的信号，
// 悄悄夹到 100 分会让「所有书都是满分」这种退化一直不被发现。
func (p curationPayload) normalize(allowed []string) (Curation, error) {
	decision, err := normalizeDecision(p.Decision)
	if err != nil {
		return Curation{}, err
	}
	if p.Score < 0 || p.Score > 100 {
		return Curation{}, fmt.Errorf("%w：score %d 超出 0..100", ErrBadResponse, p.Score)
	}
	return Curation{
		Decision: decision,
		Score:    p.Score,
		Reasons:  cleanList(p.Reasons, maxCardReasons),
		Category: pickCategory(p.Category, allowed),
		Tags:     cleanList(p.Tags, maxCardTags),
	}, nil
}

// normalizeDecision 把模型的 decision 归到三个常量之一（大小写与空白宽容）。
func normalizeDecision(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case DecisionImport:
		return DecisionImport, nil
	case DecisionSkip:
		return DecisionSkip, nil
	case DecisionReview:
		return DecisionReview, nil
	default:
		return "", fmt.Errorf("%w：decision %q 不是 import/skip/review", ErrBadResponse, s)
	}
}

// cleanList 去空白、去重、截断到 n 条。
func cleanList(items []string, n int) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
		if len(out) == n {
			break
		}
	}
	return out
}

// pickCategory 只接受「库内已有分类」里的名字；不在表里一律丢弃。
//
// 不让模型自由编分类：库里没有的分类写进去，筛选器上就会出现一个只有一本书的孤岛。
func pickCategory(name string, allowed []string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(allowed) == 0 {
		return ""
	}
	for _, a := range allowed {
		if a == name {
			return name
		}
	}
	return ""
}
