package parser

import (
	"strings"

	"norvels/internal/model"
)

const preambleAsChapterChars = 2000

// keepPreambleAsChapter 把过长的前置正文补成一章，插在最前面。
//
// 前置正文已经参与过元数据抽取（书名/作者/简介都从它来），但如果没有这一层，
// 它就只是「元数据素材」而不会出现在阅读内容里——正文会凭空少一大截。
func (d *Doc) keepPreambleAsChapter(preamble string) {
	pre := d.cleanerOr().cleanContentAt(preamble, 0)
	if runeLen(pre) < preambleAsChapterChars {
		return
	}
	head := model.ParsedChapter{
		Title: "正文", VolumeIdx: -1, Content: pre, WordCount: countWords(pre),
	}
	d.Chapters = append([]model.ParsedChapter{head}, d.Chapters...)
	for i := range d.Chapters {
		d.Chapters[i].Idx = i
	}
	d.Warn("首个章节标题前有 %d 字正文，已单列为「正文」章", runeLen(pre))
}

// refineSplit 决定「候选切分要不要进一步处理」，取代原来的二选一整体回退。
//
// 旧逻辑是：章太少而正文很长 → 整本换成无标题的等分切分（或反之）。
// 那是全有或全无，会把本来正确的候选结构一起扔掉。现在分三步：
//
//  1. 候选数守卫：候选保留 ≥ 3 个说明书确实有可行的章节结构，只允许局部再切，
//     不做整体回退；
//  2. count_sanity：章数落在合理区间就什么都不做（15 章 / 4.5 万字属正常）；
//  3. 局部再切：只对离群章递归找标题，结构分不足 0.5 就原样保留，
//     非离群章一个字节都不动。
func (d *Doc) refineSplit(cands []candidate) {
	total := 0
	for _, c := range d.Chapters {
		total += runeLen(c.Content)
	}
	kept := len(d.chaptersRaw())
	sanity := countSanity(len(d.Chapters), total)
	d.Stat("count_sanity", sanity)

	// 第 3 步：先做局部再切（对任何情况都安全，因为它不改非离群章）。
	refined := d.reSplitOutliers(cands)
	if refined > 0 {
		d.Warn("检测到 %d 个离群长章，已按原文结构局部再切", refined)
		// 局部再切后章数变了，重新算一次总量用于后续判定。
		total = 0
		for _, c := range d.Chapters {
			total += runeLen(c.Content)
		}
		sanity = countSanity(len(d.Chapters), total)
	}

	if kept >= 3 || sanity >= 0.999 {
		// 候选足够或有合理章数：到此为止，绝不整体回退。
		return
	}
	// 候选少、章数也确实不合理（例如整本只有 1~2 章却几十万字）才整体兜底。
	fb := fallbackSplitDoc(d)
	if len(fb.chapters) > len(d.Chapters) {
		avg := 0
		if len(d.Chapters) > 0 {
			avg = total / len(d.Chapters)
		}
		d.Volumes, d.Chapters = fb.volumes, fb.chapters
		d.Warn("章节标记过少（候选 %d 个 / %d 章 / 平均 %d 字符），已按长度兜底切分",
			len(cands), kept, avg)
	}
}

// chaptersRaw 返回已切出的章节数（供候选数守卫使用）。
func (d *Doc) chaptersRaw() []model.ParsedChapter { return d.Chapters }

// reSplitOutliers 只对离群长章递归找标题并再切，返回被再切的章数。
//
// 关键约束（「do not shred prose」）：子切分必须切出 ≥ 2 段、且新候选的
// 结构分 ≥ outlierAcceptScore 才采纳；否则原章一个字都不动。
func (d *Doc) reSplitOutliers(cands []candidate) int {
	if len(d.Chapters) == 0 {
		return 0
	}
	contents := make([]string, len(d.Chapters))
	for i, c := range d.Chapters {
		contents[i] = c.Content
	}
	idx := findOutliers(contents)
	if len(idx) == 0 {
		return 0
	}
	rebuilt := make([]model.ParsedChapter, 0, len(d.Chapters))
	changed := 0
	for i, ch := range d.Chapters {
		if !containsInt(idx, i) {
			rebuilt = append(rebuilt, ch)
			continue
		}
		parts, ok := splitOutlierContent(ch, d.cleanerOr())
		if !ok {
			rebuilt = append(rebuilt, ch)
			continue
		}
		changed++
		rebuilt = append(rebuilt, parts...)
	}
	if changed == 0 {
		return 0
	}
	for i := range rebuilt {
		rebuilt[i].Idx = i
	}
	d.Chapters = rebuilt
	return changed
}

// containsInt 判断下标是否在集合里。
func containsInt(set []int, v int) bool {
	for _, x := range set {
		if x == v {
			return true
		}
	}
	return false
}

// outlierAcceptScore 是采纳「离群章局部再切」的最低结构分。
const outlierAcceptScore = 0.5

// splitOutlierContent 对单个离群章递归找标题并切分。
//
// 返回 (新章列表, 是否采纳)。两个条件同时满足才采纳：
// 切出 ≥2 段（确实切开了），且结构分 ≥ outlierAcceptScore（标题确实像章节标记）。
func splitOutlierContent(ch model.ParsedChapter, cl *cleaner) ([]model.ParsedChapter, bool) {
	text := ch.Content
	cands := scanCandidates(text)
	if len(cands) < 2 {
		if extra := scanEmbedded(text); len(extra) > 0 {
			cands = mergeCandidates(cands, extra)
		}
	}
	cands = validateCandidates(cands)
	if len(cands) < 2 {
		return nil, false
	}
	if structureScore(cands, len(text)) < outlierAcceptScore {
		return nil, false
	}
	res := buildFromCandidates(text, cands, cl)
	if len(res.chapters) < 2 {
		return nil, false
	}
	out := make([]model.ParsedChapter, 0, len(res.chapters))
	for i, c := range res.chapters {
		title := c.Title
		content := c.Content
		if i == 0 {
			// 首段沿用原标题，避免丢失「这是哪一章」的信息。
			title = ch.Title
			// 首个标题之前的正文（preamble）不能丢：它是原文的一部分，
			// 只是缺少标题。把它前置到首段开头。
			if pre := cl.cleanContentAt(res.preamble, 0); strings.TrimSpace(pre) != "" {
				if content == "" {
					content = pre
				} else {
					content = pre + "\n" + content
				}
			}
		}
		out = append(out, model.ParsedChapter{
			Title: title, VolumeIdx: ch.VolumeIdx, Content: content, WordCount: countWords(content),
		})
	}
	return out, true
}

// structureScore 评估一批候选标题的「结构可信度」0~1。
//
// 依据：候选密度（每千字符几个标题）+ 强特征占比。纯段落流里偶尔出现的
// 「第N章」引用会让密度极低，分数随之偏低——正好被 0.5 的门槛挡掉。
func structureScore(cands []candidate, chars int) float64 {
	if len(cands) < 2 || chars <= 0 {
		return 0
	}
	strong := 0
	for _, c := range cands {
		if c.h.strong || c.h.kind == headVolume {
			strong++
		}
	}
	// 每千字 0.3 个标题（约 3300 字一章）视为「密度足够」，达到即满分。
	// 密度权重更高：段落流里偶尔出现的「见第三章」引用会让密度极低，
	// 仅靠强特征占比是识别不出来的（那些引用本身也是强特征）。
	const saturatedDensity = 0.3
	density := float64(len(cands)) / (float64(chars) / 1000.0)
	densityScore := density / saturatedDensity
	if densityScore > 1 {
		densityScore = 1
	}
	strongScore := float64(strong) / float64(len(cands))
	return 0.7*densityScore + 0.3*strongScore
}

// promoteVolumeOnly 处理「只有卷标记、没有章标记」的书。
//
// 典型语料：按部出版的合集（「第一部」「第二部」…）或只有分卷标记的段落流。
// 这类书写里卷就是最高层级的分段单位；照常只建卷的话，卷下没有章标记，
// 整本书会塌成 1 章（或 0 章）。这里把每个卷当作一章来切。
//
// 触发条件严格：候选里必须一个可用章标记都没有，且有 ≥2 个卷标记。
// 一旦存在章标记，说明书本来就是按章写的，交给正常流程。
