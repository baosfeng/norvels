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
	if promoted, ok := promoteVolumeOnly(d, res, cands); ok {
		// 「卷即章」已经是最贴合原文的切法：单卷仍可能偏大，但那由
		// postprocess 的超长章再切处理（标题保留「卷名（续N）」），
		// 不要在这里整体换成无标题的等分切分。
		d.Volumes, d.Chapters = promoted.volumes, promoted.chapters
		d.Warnings = append(d.Warnings, promoted.warnings...)
		return nil
	}
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

// promoteVolumeOnly 处理「只有卷标记、没有章标记」的书。
//
// 典型语料：按部出版的合集（「第一部」「第二部」…）或只有分卷标记的段落流。
// 这类书写里卷就是最高层级的分段单位；照常只建卷的话，卷下没有章标记，
// 整本书会塌成 1 章（或 0 章）。这里把每个卷当作一章来切。
//
// 触发条件严格：候选里必须一个可用章标记都没有，且有 ≥2 个卷标记。
// 一旦存在章标记，说明书本来就是按章写的，交给正常流程。
func promoteVolumeOnly(d *Doc, res splitResult, cands []candidate) (splitResult, bool) {
	vols, chapters := 0, 0
	for _, c := range cands {
		switch c.h.kind {
		case headVolume:
			vols++
		case headChapter:
			if c.h.hasNum || c.h.strong {
				chapters++
			}
		}
	}
	// 与兜底切分同一口径：章标记少于 3 个说明书其实不是「按章写」的结构，
	// 零星的一两个章标记通常是正文里的误判或卷内的孤立小标题。
	if chapters >= 3 || vols < 2 {
		return splitResult{}, false
	}
	cl := d.cleanerOr()
	out := splitResult{preamble: res.preamble}
	for i, c := range cands {
		if c.h.kind != headVolume {
			continue
		}
		end := len(d.Text)
		if i+1 < len(cands) {
			end = cands[i+1].start
		}
		if end < c.bodyStart {
			end = c.bodyStart
		}
		body := cl.cleanContentAt(d.Text[c.bodyStart:end], c.bodyStart)
		title := c.h.volTitle
		if title == "" {
			title = "未命名卷"
		}
		out.chapters = append(out.chapters, model.ParsedChapter{
			Title: title, VolumeIdx: -1, Content: body, WordCount: countWords(body),
		})
	}
	if len(out.chapters) < 2 {
		return splitResult{}, false
	}
	// 升级为「卷即章」后不再需要卷分组（每卷恰好对应一章）。
	out.volumes = nil
	out.warnings = append(out.warnings, "本文只有分卷标记、没有章节标记，已按卷切章")
	return out, true
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
