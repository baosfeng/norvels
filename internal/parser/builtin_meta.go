package parser

import (
	"path/filepath"
	"strings"
)

// ---------- Meta: filename ----------

// filenameMeta 从文件名推断书名与作者（来源标记《》/【】、「作者：」等）。
//
// 权重最高：文件名是人工整理过的，通常比正文头部可靠。
type filenameMeta struct{}

func (filenameMeta) Name() string { return "filename" }
func (filenameMeta) Weight() int  { return 80 }

func (filenameMeta) Extract(d *Doc) (Meta, int) {
	title, author, fromMark := titleFromFilename(d.Name)
	title = enforceCleanTitle(d, title)
	m := Meta{}
	if title != "" {
		m.Title = title
		m.TitleScore = 90
		if fromMark {
			m.TitleScore = 95
		}
	}
	if author != "" {
		m.Author = author
		m.AuthorScore = 95
	}
	return m, bestScore(m)
}

// ---------- Meta: preamble ----------

// preambleMeta 从正文头部（第一个标题之前）的元信息块抽书名 / 作者 / 简介。
type preambleMeta struct{}

func (preambleMeta) Name() string { return "preamble" }
func (preambleMeta) Weight() int  { return 60 }

func (preambleMeta) Extract(d *Doc) (Meta, int) {
	pre := d.preamble()
	title, author, intro := metaFromPreamble(pre)
	if author == "" {
		// 头部元信息块可能被超长的「书名/简介」挤到很后面，也可能写在文末版权块里。
		// 这时用「短行 + 作者标记」的特征在文档两端再捞一次。
		author = authorFromEdges(d.Text)
	}
	m := Meta{}
	if title != "" {
		m.Title, m.TitleScore = title, 70
	}
	if author != "" {
		m.Author, m.AuthorScore = author, 70
	}
	if intro != "" {
		m.Intro, m.IntroScore = intro, 80
	}
	return m, bestScore(m)
}

// preamble 返回首个章节标题之前的正文；没有候选或该段为空时取全文头部。
func (d *Doc) preamble() string {
	if d.pre != "" {
		return d.pre
	}
	if len(d.cands) > 0 && d.cands[0].start > 0 {
		if t := d.Text[:d.cands[0].start]; strings.TrimSpace(t) != "" {
			d.pre = t
			return d.pre
		}
	}
	d.pre = headOfText(d.Text, 4000)
	return d.pre
}

// ---------- Meta: heading-reverse ----------

// headingReverseMeta 在头部元信息缺失时由章节标题反推书名。
//
// 适用场景：「书名 第N章 标题」这类把书名塞进每行标题的排版，头部干净得
// 抽不出任何信息。取出现次数最多的前缀作为书名。
type headingReverseMeta struct{}

func (headingReverseMeta) Name() string { return "heading-reverse" }
func (headingReverseMeta) Weight() int  { return 40 }

func (headingReverseMeta) Extract(d *Doc) (Meta, int) {
	cands := d.titleCandidates()
	if len(cands) == 0 {
		return Meta{}, 0
	}
	counts := make(map[string]int, 4)
	for _, c := range cands {
		if c.h.volTitle == "" {
			continue
		}
		counts[c.h.volTitle]++
		if len(counts) > 32 {
			break
		}
	}
	best, bestN := "", 0
	for k, n := range counts {
		if n > bestN || (n == bestN && len(k) < len(best)) {
			best, bestN = k, n
		}
	}
	// 只出现一两次的前缀更可能是正文里的误判，不足以当书名。
	if bestN < 3 {
		return Meta{}, 0
	}
	best = cleanTitleText(best)
	if best == "" {
		return Meta{}, 0
	}
	out := Meta{Title: best, TitleScore: 55}
	return out, bestScore(out)
}

// ---------- Meta: filename-stem ----------

// stemMeta 是最低优先级的兜底：把文件名主干当书名，作者留空。
//
// 权重最低，所以只要任何一个策略给出了书名就会被覆盖；它的价值在于保证
// 书名永远不为空（避免整本书因为抽不到书名而被丢弃）。
type stemMeta struct{}

func (stemMeta) Name() string { return "stem" }
func (stemMeta) Weight() int  { return 10 }

func (stemMeta) Extract(d *Doc) (Meta, int) {
	base := filepath.Base(d.Name)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	base = cleanTitleText(stripFileDecor(base))
	if base == "" {
		return Meta{}, 0
	}
	out := Meta{Title: base, TitleScore: 20}
	return out, bestScore(out)
}

// registerBuiltinMetas 注册全部内置元数据策略。
func registerBuiltinMetas() {
	RegisterMeta(filenameMeta{})
	RegisterMeta(preambleMeta{})
	RegisterMeta(headingReverseMeta{})
	RegisterMeta(stemMeta{})
}

// runMetas 让每个元数据策略投票，按「字段分 × 策略权重」取最高分。
//
// 逐字段投票的意义：文件名可能只给得出书名、正文头部只给得出作者，
// 两者可以同时生效，不需要任何一方全胜。
func runMetas(d *Doc, extra []MetaExtractor) {
	list := append(Metas(), extra...)
	hit := make([]string, 0, len(list))
	var m Meta
	for _, ex := range list {
		name := ex.Name()
		var got Meta
		var weight int
		err := guard(d, "meta", name, func() error {
			got, weight = ex.Extract(d)
			return nil
		})
		if err != nil {
			continue
		}
		before := m
		mergeMeta(&m, got, weight)
		if m != before {
			hit = append(hit, name)
		}
	}
	d.Meta = m
	d.Stat("meta_hits", hit)
}

// bestScore 返回三个字段里的最高分，作为「这个策略在本篇命中了多少」的度量。
//
// 注意它和字段分不是一回事：Extract 的第二个返回值表示策略整体的适用度
// （用于诊断与降级判定），字段分才参与逐字段投票——某个策略只抽到作者时，
// 适用度依然是正数，作者分不会被权重压成 0。
func bestScore(m Meta) int {
	best := m.TitleScore
	if m.AuthorScore > best {
		best = m.AuthorScore
	}
	if m.IntroScore > best {
		best = m.IntroScore
	}
	return best
}
