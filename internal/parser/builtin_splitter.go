package parser

import (
	"fmt"
	"strings"

	"norvels/internal/model"
)

// titleCandidates 返回本文本的标题候选（一次扫描，各策略复用）。
func (d *Doc) titleCandidates() []candidate {
	if d.scanned {
		return d.cands
	}
	d.scanned = true
	d.cands = scanCandidates(d.Text)
	if len(d.cands) < 3 {
		// 换行极少的文件：章节标记可能埋在超长行内部。
		if extra := scanEmbedded(d.Text); len(extra) > 0 {
			d.cands = mergeCandidates(d.cands, extra)
		}
	}
	d.cands = validateCandidates(d.cands)
	return d.cands
}

// cleanerOr 返回噪声清洗器（noise 过滤器被关闭时退化为「全规则」清洗器）。
func (d *Doc) cleanerOr() *cleaner {
	if d.cl != nil {
		return d.cl
	}
	return fullCleaner
}

// ---------- Splitter: di-unit ----------

// diUnitSplitter 处理「第N章 / 第N节 / 第N回 / 第N篇 / 第N话」这一主形态。
//
// Detect：强特征（第N章）候选占比越高越可信；卷标记也算支持。
type diUnitSplitter struct{}

func (diUnitSplitter) Name() string { return "di-unit" }

func (diUnitSplitter) Detect(d *Doc) float64 {
	cands := d.titleCandidates()
	if len(cands) == 0 {
		return 0
	}
	strong := 0
	for _, c := range cands {
		if c.h.kind == headVolume || (c.h.strong && c.h.kind == headChapter && strings.HasPrefix(c.h.marker, "第")) {
			strong++
		}
	}
	if strong == 0 {
		return 0
	}
	ratio := float64(strong) / float64(len(cands))
	// 至少 3 个强候选才认领，避免正文里零星的「第三章」把切分带偏。
	if strong < 3 {
		ratio *= 0.3
	}
	return ratio
}

func (diUnitSplitter) Split(d *Doc) error {
	return splitWithCandidates(d, d.titleCandidates())
}

// ---------- Splitter: special ----------

// specialSplitter 处理全文只由序章 / 楔子 / 引子 / 尾声 / 番外这类标记分章的书。
type specialSplitter struct{}

func (specialSplitter) Name() string { return "special" }

func (specialSplitter) Detect(d *Doc) float64 {
	cands := d.titleCandidates()
	if len(cands) == 0 {
		return 0
	}
	special := 0
	for _, c := range cands {
		if c.h.kind == headSpecial {
			special++
		}
	}
	if special < 2 {
		return 0
	}
	return float64(special) / float64(len(cands))
}

func (specialSplitter) Split(d *Doc) error {
	return splitWithCandidates(d, d.titleCandidates())
}

// ---------- Splitter: numeric ----------

// numericSplitter 处理「1、标题」「0001」这类纯数字章节行。
type numericSplitter struct{}

func (numericSplitter) Name() string { return "numeric" }

func (numericSplitter) Detect(d *Doc) float64 {
	cands := d.titleCandidates()
	if len(cands) == 0 {
		return 0
	}
	num := 0
	for _, c := range cands {
		if c.h.strong {
			continue
		}
		if c.h.hasNum {
			num++
		}
	}
	if num < 2 {
		return 0
	}
	return float64(num) / float64(len(cands))
}

func (numericSplitter) Split(d *Doc) error {
	return splitWithCandidates(d, d.titleCandidates())
}

// ---------- Splitter: en-chapter ----------

// enChapterSplitter 处理「Chapter N」这类英文标记。
type enChapterSplitter struct{}

func (enChapterSplitter) Name() string { return "en-chapter" }

func (enChapterSplitter) Detect(d *Doc) float64 {
	cands := d.titleCandidates()
	if len(cands) == 0 {
		return 0
	}
	n := 0
	for _, c := range cands {
		if strings.HasPrefix(strings.ToLower(c.h.marker), "chapter") {
			n++
		}
	}
	if n < 2 {
		return 0
	}
	return float64(n) / float64(len(cands))
}

func (enChapterSplitter) Split(d *Doc) error {
	return splitWithCandidates(d, d.titleCandidates())
}

// ---------- Splitter: unmarked ----------

// unmarkedSplitter 是兜底策略：Detect 恒为最低正分，永远排在最后。
type unmarkedSplitter struct{}

func (unmarkedSplitter) Name() string { return "unmarked" }

func (unmarkedSplitter) Detect(d *Doc) float64 { return 0.01 }

func (unmarkedSplitter) Split(d *Doc) error {
	res := fallbackSplitDoc(d)
	d.Volumes, d.Chapters = nil, nil
	d.Volumes, d.Chapters = res.volumes, res.chapters
	d.Warnings = append(d.Warnings, res.warnings...)
	return nil
}

// registerBuiltinSplitters 注册全部内置切章策略。
func registerBuiltinSplitters() {
	RegisterSplitter(diUnitSplitter{})
	RegisterSplitter(specialSplitter{})
	RegisterSplitter(numericSplitter{})
	RegisterSplitter(enChapterSplitter{})
	RegisterSplitter(unmarkedSplitter{})
}

// runSplitter 用 Detect 打分挑选策略；forced 非空时跳过打分。
//
// 降级语义：选中的策略失败（panic / error）时回退到兜底切分，
// 保证「至少能入库」，而不是整篇解析失败。
func runSplitter(d *Doc, forced string, extra []Splitter) {
	list := append(Splitters(), extra...)
	clearSplit(d)
	if forced != "" {
		for _, s := range list {
			if s.Name() == forced {
				if guard(d, "splitter", forced, func() error { return s.Split(d) }) == nil && hasChapters(d) {
					d.Stat("splitter", forced)
					return
				}
				d.Warn("强制切章策略 %q 未产出章节，改用兜底切分", forced)
				break
			}
		}
		d.Stat("splitter_unknown_forced", forced)
	}
	best, bestScore := "", -1.0
	for _, s := range list {
		if s.Name() == "unmarked" {
			continue
		}
		name := s.Name()
		var score float64
		err := guard(d, "splitter", name, func() error { score = s.Detect(d); return nil })
		if err != nil || score <= 0 {
			continue
		}
		if score > bestScore {
			best, bestScore = name, score
		}
	}
	if best != "" {
		for _, s := range list {
			if s.Name() != best {
				continue
			}
			clearSplit(d)
			if guard(d, "splitter", best, func() error { return s.Split(d) }) == nil && hasChapters(d) {
				d.Stat("splitter", best)
				d.Stat("splitter_score", bestScore)
				return
			}
			d.Warn("切章策略 %q 未产出有效章节，改用兜底切分", best)
			break
		}
	}
	clearSplit(d)
	for _, s := range list {
		if s.Name() != "unmarked" {
			continue
		}
		_ = guard(d, "splitter", "unmarked", func() error { return s.Split(d) })
		d.Stat("splitter", "unmarked")
		return
	}
}

// clearSplit 清空上一次切分的结果（策略重试时需要）。
func clearSplit(d *Doc) {
	d.Volumes, d.Chapters = nil, nil
}

// hasChapters 判断切分是否真的产出了章节。
func hasChapters(d *Doc) bool { return len(d.Chapters) > 0 }

// splitWithCandidates 用候选标题构建卷与章，再跑一次兜底判定。
func splitWithCandidates(d *Doc, cands []candidate) error {
	if len(cands) == 0 {
		return fmt.Errorf("no candidates")
	}
	res := buildFromCandidatesDoc(d, cands)
	d.Volumes, d.Chapters = res.volumes, res.chapters
	d.restarts = res.restarts
	if res.preamble != "" {
		d.pre = res.preamble
	}
	d.Warnings = append(d.Warnings, res.warnings...)
	// 切出来的章太少而正文很长（说明标题格式不规范）→ 交给兜底策略。
	total := 0
	for _, c := range d.Chapters {
		total += runeLen(c.Content)
	}
	if total > 12000 && (len(d.Chapters) <= 2 || total/len(d.Chapters) > 30000) {
		fb := fallbackSplitDoc(d)
		if len(fb.chapters) > len(d.Chapters) {
			d.Volumes, d.Chapters = fb.volumes, fb.chapters
			d.Warnings = append(d.Warnings, "章节标记过少，已按长度兜底切分")
			return nil
		}
	}
	return nil
}

// buildFromCandidatesDoc 是 buildFromCandidates 的 Doc 版本。
func buildFromCandidatesDoc(d *Doc, cands []candidate) splitResult {
	return buildFromCandidates(d.Text, cands, d.cleanerOr())
}

// fallbackSplitDoc 是 fallbackSplit 的 Doc 版本。
func fallbackSplitDoc(d *Doc) splitResult {
	return fallbackSplit(d.Text, d.cleanerOr(), fallbackOpts{Chunk: d.opts.FallbackChunkChars, Allow: d.opts.AllowFallbackSplit})
}

var _ = model.ParsedChapter{}
