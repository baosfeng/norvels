package parser

import (
	"fmt"
	"strings"

	"norvels/internal/model"
)

// buildFromCandidates 依据候选标题构建卷与章。
func buildFromCandidates(text string, cands []candidate, cl *cleaner) splitResult {
	res := splitResult{}
	if len(cands) > 0 {
		res.preamble = text[:cands[0].start]
	}
	cur := chapterCursor{vol: -1, pendingBody: -1}
	for _, c := range cands {
		if c.h.kind == headVolume {
			res.addVolume(c, &cur, text, cl)
			continue
		}
		res.addChapter(c, &cur, text, cl)
	}
	// 卷标记之后再也没有章标记：由卷收尾成章，保证正文不丢。
	if cur.pendingBody >= 0 && !cur.started && cur.pendingBody < len(text) {
		cur.title, cur.inline, cur.body, cur.started = cur.pendingTitle, "", cur.pendingBody, true
	}
	res.flushChapter(len(text), text, &cur, cl)
	return res
}

// chapterCursor 是切分过程中的游标状态。
type chapterCursor struct {
	vol          int    // 当前卷下标，-1 表示未分卷
	title        string // 当前章标题
	inline       string // 标题行里被并进来的正文
	body         int    // 当前章正文起点
	started      bool   // 是否已经开始一章
	lastNum      int    // 上一章章号（用于检测章号重置）
	pendingBody  int    // 卷标记后挂起、等待章标记认领的正文起点
	pendingTitle string // 对应的卷标题
}

// addVolume 处理一个卷标记。
func (res *splitResult) addVolume(c candidate, cur *chapterCursor, text string, cl *cleaner) {
	res.flushChapter(c.start, text, cur, cl)
	vt := c.h.volTitle
	if vt == "" {
		vt = "未命名卷"
	}
	res.volumes = append(res.volumes, model.ParsedVolume{Idx: len(res.volumes), Title: vt})
	cur.vol = len(res.volumes) - 1
	// 卷标记之后的正文先挂起：等后面第一个章标记来认领，否则由卷自己收尾。
	cur.pendingBody, cur.pendingTitle = c.bodyStart, vt
}

// addChapter 处理一个章标记。
func (res *splitResult) addChapter(c candidate, cur *chapterCursor, text string, cl *cleaner) {
	h := c.h
	if h.hasNum {
		if cur.lastNum > 10 && h.num <= 2 && len(res.chapters) > 3 {
			res.restarts++
		}
		cur.lastNum = h.num
	}
	res.flushChapter(c.start, text, cur, cl)
	cur.title = joinTitle(h.marker, h.title)
	cur.inline = h.inline
	cur.body = c.bodyStart
	if cur.pendingBody >= 0 && !cur.started {
		// 卷标记之后、本章标题之前的正文属于本卷，必须并进本章，
		// 否则这段内容不在任何章节的区间里，会被静默丢弃。
		cur.body = cur.pendingBody
	}
	cur.pendingBody, cur.pendingTitle = -1, ""
	cur.started = true
}

// flushChapter 把当前章落到 res.chapters（end 是本章正文的结束偏移）。
func (res *splitResult) flushChapter(end int, text string, cur *chapterCursor, cl *cleaner) {
	if !cur.started {
		return
	}
	body := text[cur.body:min(end, len(text))]
	content := cl.cleanContentAt(body, cur.body)
	if cur.inline != "" {
		if head := cl.cleanContent(cur.inline); head != "" {
			if content == "" {
				content = head
			} else {
				content = head + "\n" + content
			}
		}
	}
	title := cur.title
	if title == "" {
		title = "正文"
	}
	res.chapters = append(res.chapters, model.ParsedChapter{
		Title: title, VolumeIdx: cur.vol, Content: content, WordCount: countWords(content),
	})
	cur.started = false
}

// fallbackOpts 是兜底切分的两个可配参数。
type fallbackOpts struct {
	Chunk int  // 每章字符数
	Allow bool // 是否允许按长度等分
}

// fallbackSplit 在没有任何章节标记时兜底：整篇单章或等长切分。
func fallbackSplit(text string, cl *cleaner, opt fallbackOpts) splitResult {
	res := splitResult{preamble: headOfText(text, 4000)}
	content := cl.cleanContentAt(text, 0)
	if content == "" {
		return res
	}
	chunk := opt.Chunk
	if chunk <= 0 {
		chunk = 8000
	}
	if !opt.Allow || runeLen(content) <= chunk*3/2 {
		res.chapters = append(res.chapters, model.ParsedChapter{
			Title: "全文", VolumeIdx: -1, Content: content, WordCount: countWords(content),
		})
		res.warnings = append(res.warnings, "未发现章节标记，按单章处理")
		return res
	}
	var b strings.Builder
	b.Grow(chunk * 3)
	n := 0
	flush := func() {
		if b.Len() == 0 {
			return
		}
		c := b.String()
		n++
		res.chapters = append(res.chapters, model.ParsedChapter{
			Title: fmt.Sprintf("第%d节", n), VolumeIdx: -1, Content: c, WordCount: countWords(c),
		})
		b.Reset()
	}
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if runeLen(b.String()) >= chunk {
			flush()
		}
	}
	flush()
	res.warnings = append(res.warnings, "未发现章节标记，按长度等分切分")
	return res
}

// postProcess 合并过短/空章节、修正卷下标、重排章节序号。
func postProcess(res *splitResult, minLen int) {
	if minLen < 0 {
		minLen = 0
	}
	merged := make([]model.ParsedChapter, 0, len(res.chapters))
	for _, c := range res.chapters {
		if isEmptyChapter(c) {
			// 空壳章节：标题并到上一章，避免丢章节名；正文本来就没有，直接丢。
			carryTitleToPrev(merged, c)
			continue
		}
		if len(merged) > 0 && len(c.Content) < minLen && !isSpecialTitle(c.Title) {
			prev := &merged[len(merged)-1]
			if c.Title != "" && c.Title != "正文" {
				prev.Content += "\n" + c.Title
				prev.WordCount += countWords(c.Title)
			}
			prev.Content += "\n" + c.Content
			prev.WordCount += c.WordCount
			continue
		}
		merged = append(merged, c)
	}
	if len(merged) == 0 {
		res.chapters = nil
		res.volumes = nil
		return
	}
	// 卷裁剪与重排
	used := make(map[int]bool, len(res.volumes))
	for _, c := range merged {
		if c.VolumeIdx >= 0 && c.VolumeIdx < len(res.volumes) {
			used[c.VolumeIdx] = true
		}
	}
	remap := make(map[int]int, len(res.volumes))
	vols := make([]model.ParsedVolume, 0, len(res.volumes))
	for i, v := range res.volumes {
		if !used[i] {
			continue
		}
		remap[i] = len(vols)
		v.Idx = len(vols)
		vols = append(vols, v)
	}
	for i := range merged {
		merged[i].Idx = i
		if merged[i].VolumeIdx >= 0 {
			if nv, ok := remap[merged[i].VolumeIdx]; ok {
				merged[i].VolumeIdx = nv
			} else {
				merged[i].VolumeIdx = -1
			}
		}
	}
	res.chapters = merged
	res.volumes = vols
}

// headOfText 取文本开头 n 字节（不足则全文），用于元数据/合集判定。
func headOfText(text string, n int) string {
	if len(text) <= n {
		return text
	}
	cut := n
	if i := strings.LastIndexByte(text[:n], '\n'); i > 0 {
		cut = i
	}
	return text[:cut]
}

// isEmptyChapter 判断一章是否没有可读内容。
//
// 不能只判 Content == ""：噪声清洗 + 超长章再切之后，空壳章节经常剩下一串
// 装饰符（星号、全角等号、破折号、波浪线…）。这类内容 TrimSpace 去不掉，
// 但统计字数为 0——用字数判定才是「有没有可读内容」的准确口径。
func isEmptyChapter(c model.ParsedChapter) bool {
	if strings.TrimSpace(c.Content) == "" {
		return true
	}
	return countWords(c.Content) == 0
}

// carryTitleToPrev 把空壳章节的标题并入上一章的末尾。
//
// 只做标题搬运：正文为空，没有内容可合。首章位置的空壳没有上一章可并，
// 其标题会被丢弃——这时正文本来就还没开始，留标题反而会造出孤立空章。
func carryTitleToPrev(merged []model.ParsedChapter, c model.ParsedChapter) {
	if len(merged) == 0 || c.Title == "" || c.Title == "正文" {
		return
	}
	prev := &merged[len(merged)-1]
	prev.Content += "\n" + c.Title
	prev.WordCount += countWords(c.Title)
}
