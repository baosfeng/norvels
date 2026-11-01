package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// 本文件是 AI 审核的输入构造（设计 §6「输入只含差异段落抽样」）：
// 按章 idx 对齐新旧两版正文，挑出正文确实不同的章，做成带上下文的差异片段。
//
// # 为什么按 idx 对齐
//
// 两版是「同一本书的两个文件」，章节序号是它们唯一稳定的对应关系：标题可能被重命名、
// 卷可能被重排，但第 7 章对第 7 章。只在两边都存在的 idx 上比对，缺章（多出来 /
// 少掉的章）不进片段——那是「完整性」维度，规则对比已经量化过了。
//
// # 为什么有上限
//
// 一次审核发生在后台单线程 worker 里，而正文解压是全书最贵的操作（8 MB 块 + 码本解码）。
// 比对章数封顶 maxReviewScanChapters（等距抽样），片段数封顶 maxReviewSegments；
// 两端都确定性采样（同样的两版书每次得到同样的输入），便于复盘与测试。

// 审核输入的体量上限（与 internal/ai 的提示词上限同口径：50 段 / 每段 1000 rune /
// 上下文 400 rune）。store 侧先截断一次，ai 侧还会再截断，两层都不会超发正文。
const (
	maxReviewSegments     = 50
	maxReviewScanChapters = 600
	maxSegmentRunes       = 1000
	maxContextRunes       = 400
)

// 句末标点：切「前后各一句」的上下文用（中文句读 + 换行）。
const sentenceEnders = "。！？；…\n"

// ReviewSegment 是同一章两版正文的一处差异片段。
//
// store 不依赖 ai 包，因此这里用自己的形状；调用方（api 的 worker）把它翻成
// ai.Segment{Context, A: Old, B: New}。
type ReviewSegment struct {
	Idx     int    `json:"idx"`     // 章节序号（0 基，与 chapters.idx 同源）
	Context string `json:"context"` // 两版共有的周边文字（前一句 + 后一句）
	Old     string `json:"old"`     // 旧版该处正文（已截断）
	New     string `json:"new"`     // 新版该处正文（已截断）
}

// ReviewDiff 是一次新旧版本正文对齐的结果（不进库，只给 worker 与日志用）。
type ReviewDiff struct {
	OldChapters int             // 旧版本章数
	NewChapters int             // 新版本章数
	Compared    int             // 实际逐章比对过的章数（对齐后按上限抽样）
	Differing   int             // 其中正文确实不同的章数
	Segments    []ReviewSegment // 抽样后的差异片段（≤ maxReviewSegments）
}

// SampleReviewDiff 对齐新旧两版正文并抽取差异片段。
//
// 只读正文、不写库；两版任一不存在时返回空结果（书被删了，审核自然没有输入）。
func (s *Store) SampleReviewDiff(ctx context.Context, oldBookID, newBookID int64) (ReviewDiff, error) {
	if oldBookID <= 0 || newBookID <= 0 {
		return ReviewDiff{}, fmt.Errorf("%w: 审核对照的书 id 非法（old=%d new=%d）",
			ErrInvalidArgument, oldBookID, newBookID)
	}
	oldSpans, err := s.reviewSpans(ctx, oldBookID)
	if err != nil {
		return ReviewDiff{}, err
	}
	newSpans, err := s.reviewSpans(ctx, newBookID)
	if err != nil {
		return ReviewDiff{}, err
	}
	diff := ReviewDiff{OldChapters: len(oldSpans), NewChapters: len(newSpans)}
	pairs := limitChapterPairs(alignChapterSpans(oldSpans, newSpans), maxReviewScanChapters)
	diff.Compared = len(pairs)
	diffs, err := s.collectChapterDiffs(ctx, pairs)
	if err != nil {
		return ReviewDiff{}, err
	}
	diff.Differing = len(diffs)
	diff.Segments = sampleSegments(diffs, maxReviewSegments)
	return diff, nil
}

// reviewSpan 是一章正文在正文库里的定位。
type reviewSpan struct {
	id      int64 // chapters.id（解压失败时用来定位）
	idx     int
	blockID int64
	off     int
	length  int
}

// reviewSpans 读一本书的章节定位，按 idx 升序；同一 idx 只取第一行（与目录口径一致）。
func (s *Store) reviewSpans(ctx context.Context, bookID int64) ([]reviewSpan, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, idx, block_id, block_off, block_len FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: list chapter spans %d: %w", bookID, err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	seen := make(map[int]struct{}, 64)
	out := make([]reviewSpan, 0, 64)
	for rows.Next() {
		var sp reviewSpan
		if err := rows.Scan(&sp.id, &sp.idx, &sp.blockID, &sp.off, &sp.length); err != nil {
			return nil, fmt.Errorf("store: scan chapter span: %w", err)
		}
		if _, dup := seen[sp.idx]; dup {
			continue
		}
		seen[sp.idx] = struct{}{}
		out = append(out, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list chapter spans %d: %w", bookID, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].idx < out[j].idx })
	return out, nil
}

// chapterPair 是同一 idx 上的两版章节定位。
type chapterPair struct {
	idx int
	old reviewSpan
	new reviewSpan
}

// alignChapterSpans 按 idx 取交集（两边都有的章才可比）。
func alignChapterSpans(oldSpans, newSpans []reviewSpan) []chapterPair {
	byIdx := make(map[int]reviewSpan, len(newSpans))
	for _, sp := range newSpans {
		byIdx[sp.idx] = sp
	}
	out := make([]chapterPair, 0, len(oldSpans))
	for _, sp := range oldSpans {
		nsp, ok := byIdx[sp.idx]
		if !ok {
			continue
		}
		out = append(out, chapterPair{idx: sp.idx, old: sp, new: nsp})
	}
	return out
}

// limitChapterPairs 把待比对章数压到上限内（等距抽样，确定性）。
func limitChapterPairs(pairs []chapterPair, max int) []chapterPair {
	if max <= 0 || len(pairs) <= max {
		return pairs
	}
	out := make([]chapterPair, 0, max)
	for i := 0; i < max; i++ {
		out = append(out, pairs[i*len(pairs)/max])
	}
	return out
}

// chapterDiff 是一章两版的原始正文（用于构造片段）。
type chapterDiff struct {
	idx int
	old string
	new string
}

// collectChapterDiffs 逐章比对，返回正文不同的那些章（按 idx 升序）。
func (s *Store) collectChapterDiffs(ctx context.Context, pairs []chapterPair) ([]chapterDiff, error) {
	cache := &blockTextCache{st: s}
	out := make([]chapterDiff, 0, 16)
	for _, p := range pairs {
		oldText, err := cache.textAt(ctx, p.old)
		if err != nil {
			return nil, err
		}
		newText, err := cache.textAt(ctx, p.new)
		if err != nil {
			return nil, err
		}
		if sameChapterText(oldText, newText) {
			continue
		}
		out = append(out, chapterDiff{idx: p.idx, old: oldText, new: newText})
	}
	return out, nil
}

// blockTextCache 是「单块」正文缓存。
//
// 只缓存最近一块而不是全书的块：章按 idx 升序访问，相邻章基本落在同一块里，
// 单块缓存就能把重复解压压到接近 0，而内存上限只有一块（BlockTargetBytes 8 MB）。
type blockTextCache struct {
	st      *Store
	blockID int64
	text    []byte
}

// textAt 取一章正文（空章返回空串）。
func (c *blockTextCache) textAt(ctx context.Context, sp reviewSpan) (string, error) {
	if sp.blockID <= 0 {
		if sp.length != 0 {
			return "", fmt.Errorf("%w: chapter %d has length %d but no block",
				ErrCorruptContent, sp.id, sp.length)
		}
		return "", nil
	}
	if c.blockID != sp.blockID || c.text == nil {
		block, err := c.st.loadBlockText(ctx, sp.blockID, sp.id)
		if err != nil {
			return "", err
		}
		c.blockID, c.text = sp.blockID, block
	}
	text, err := chapterAt(c.text, sp.off, sp.length)
	if err != nil {
		return "", fmt.Errorf("store: chapter %d: %w", sp.id, err)
	}
	return text, nil
}

// sameChapterText 判断两版正文是否「实质相同」：忽略所有空白与换行。
//
// 只差排版（多插空行 / 全半角空格）的两版不算差异：那是清洗器的活，不是裁决的活；
// 把它们当成差异只会让 50 段配额被排版噪声占满。
func sameChapterText(a, b string) bool {
	return squashSpaces(a) == squashSpaces(b)
}

// blankRunes 是「排版差异」里要忽略的空白字符（含全角空格与换行）。
const blankRunes = " \t\n\r　"

// squashSpaces 去掉全部空白（含全角空格与换行）。
func squashSpaces(s string) string {
	if !strings.ContainsAny(s, blankRunes) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(blankRunes, r) {
			return -1
		}
		return r
	}, s)
}

// sampleSegments 把差异章压成不超过 max 个片段（等距抽样）。
func sampleSegments(diffs []chapterDiff, max int) []ReviewSegment {
	if len(diffs) == 0 {
		return []ReviewSegment{}
	}
	if max <= 0 || len(diffs) <= max {
		out := make([]ReviewSegment, 0, len(diffs))
		for _, d := range diffs {
			out = append(out, buildSegment(d))
		}
		return out
	}
	out := make([]ReviewSegment, 0, max)
	for i := 0; i < max; i++ {
		out = append(out, buildSegment(diffs[i*len(diffs)/max]))
	}
	return out
}

// buildSegment 构造一章的差异片段：共有前缀/后缀当上下文，差异从第一处不同开始截断。
func buildSegment(d chapterDiff) ReviewSegment {
	oldR := []rune(d.old)
	newR := []rune(d.new)
	prefix := commonPrefixRunes(oldR, newR)
	suffix := commonSuffixRunes(oldR, newR, prefix)
	return ReviewSegment{
		Idx:     d.idx,
		Context: diffContext(oldR[:prefix], oldR[len(oldR)-suffix:]),
		Old:     truncateRunes(string(oldR[prefix:len(oldR)-suffix]), maxSegmentRunes),
		New:     truncateRunes(string(newR[prefix:len(newR)-suffix]), maxSegmentRunes),
	}
}

// commonPrefixRunes 返回两段文本的公共前缀长度（rune 计）。
func commonPrefixRunes(a, b []rune) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// commonSuffixRunes 返回公共后缀长度，且不与前缀重叠（避免同一段文字既是前缀又是后缀）。
func commonSuffixRunes(a, b []rune, prefix int) int {
	n := min(len(a), len(b)) - prefix
	if n <= 0 {
		return 0
	}
	count := 0
	for count < n && a[len(a)-1-count] == b[len(b)-1-count] {
		count++
	}
	return count
}

// diffContext 把「前一句 + 后一句」拼成上下文（两版共有，只用来标位置）。
//
// 两段都取不到时返回空串：宁可少给上下文，也不拿差异本身当「共有周边」。
func diffContext(prefix, suffix []rune) string {
	prev := lastSentence(string(prefix))
	next := firstSentence(string(suffix))
	switch {
	case prev == "" && next == "":
		return ""
	case prev == "":
		return truncateRunes(next, maxContextRunes)
	case next == "":
		return truncateRunes(prev, maxContextRunes)
	}
	return truncateRunes(prev+"……"+next, maxContextRunes)
}

// lastSentence 取文本最后**一句完整的**话（不含句末标点）；没有标点时返回尾部至多 200 rune。
//
// 先剥掉尾部的句末标点再找上一处标点：差异正好落在句号之后时（前缀以「。」结尾），
// 直接回头找标点会得到空串，把「前一句」整个丢掉——那正是最该给模型看的位置。
func lastSentence(s string) string {
	runes := trimEndRunes([]rune(strings.TrimSpace(s)), sentenceEnders+" ")
	if len(runes) == 0 {
		return ""
	}
	for i := len(runes) - 1; i >= 0; i-- {
		if strings.ContainsRune(sentenceEnders, runes[i]) {
			return strings.TrimSpace(string(runes[i+1:]))
		}
	}
	return tailRunes(runes, 200)
}

// trimEndRunes 去掉尾部属于 cutset 的字符（用于剥句末标点与空白）。
func trimEndRunes(runes []rune, cutset string) []rune {
	for len(runes) > 0 && strings.ContainsRune(cutset, runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	return runes
}

// firstSentence 取文本第一句（含句末标点）；没有标点时返回头部至多 200 rune。
func firstSentence(s string) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) == 0 {
		return ""
	}
	for i, r := range runes {
		if strings.ContainsRune(sentenceEnders, r) {
			return strings.TrimSpace(string(runes[:i+1]))
		}
	}
	return headRunes(runes, 200)
}

// tailRunes 取末尾至多 n 个 rune。
func tailRunes(runes []rune, n int) string {
	if n <= 0 || len(runes) <= n {
		return string(runes)
	}
	return string(runes[len(runes)-n:])
}

// headRunes 取开头至多 n 个 rune。
func headRunes(runes []rune, n int) string {
	if n <= 0 || len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n])
}
