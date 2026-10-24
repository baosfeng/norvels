package parser

import (
	"fmt"
	"strings"

	"norvels/internal/model"
)

// maxChapterBytes 是单章正文的字节上限。
//
// 与 MaxChapterChars 配合使用：中文 30000 字约 90KB，而存储/检索层希望单章
// 不超过 80KB，所以取两者中更早触发的那个作为切分点。
const maxChapterBytes = 60000

// splitOversized 把超长单章按段落边界再切，标题写成「原标题（续N）」。
//
// 触发原因：部分语料的章节标题行格式不规范（混排广告、标题与正文粘连），
// 标题没被识别出来，于是一整本书被切成一个 2MB 的巨型章节。这一层是兜底，
// 保证单章体积可控、阅读端不会卡死。
func splitOversized(d *Doc) {
	limit := d.opts.MaxChapterChars
	if limit <= 0 {
		return
	}
	out := make([]model.ParsedChapter, 0, len(d.Chapters))
	splitAny := false
	for _, c := range d.Chapters {
		parts := splitChapterContent(c.Content, limit)
		if len(parts) <= 1 {
			out = append(out, c)
			continue
		}
		splitAny = true
		for i, body := range parts {
			title := c.Title
			if i > 0 {
				title = fmt.Sprintf("%s（续%d）", c.Title, i+1)
			}
			out = append(out, model.ParsedChapter{
				Title: title, VolumeIdx: c.VolumeIdx, Content: body, WordCount: countWords(body),
			})
		}
	}
	if !splitAny {
		return
	}
	for i := range out {
		out[i].Idx = i
	}
	d.Chapters = out
	d.Warn("存在超长章节（疑似标题未识别），已按段落边界切分")
}

// splitChapterContent 按段落边界把正文切到不超过 limit 个字符。
//
// 切分点优先落在空行之后；段落本身超长时退化为按字符硬切，保证一定有进展
// （否则一个没有换行的 2MB 单行会切不动）。
func splitChapterContent(content string, limit int) []string {
	if limit <= 0 || runeLen(content) <= limit {
		if len(content) <= maxChapterBytes {
			return nil
		}
	}
	var parts []string
	var b strings.Builder
	b.Grow(limit * 3)
	flush := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, strings.TrimSpace(b.String()))
		b.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		if runeLen(line) > limit {
			// 单行就超限：先把已累积的内容落盘，再按字符硬切这一行。
			flush()
			for _, seg := range hardSplit(line, limit) {
				parts = append(parts, seg)
			}
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if runeLen(b.String()) >= limit || b.Len() >= maxChapterBytes {
			flush()
		}
	}
	flush()
	if len(parts) <= 1 {
		return nil
	}
	return parts
}

// hardSplit 按字符数硬切一段超长文本。
func hardSplit(s string, limit int) []string {
	var out []string
	for runeLen(s) > limit {
		cut := truncateRunes(s, limit)
		out = append(out, cut)
		s = s[len(cut):]
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}
