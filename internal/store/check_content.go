package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"unicode/utf8"
)

// ContentCheckReport 是一次正文自查的结果。
//
// 用途：迁移（尤其 0007 把正文搬进压缩块）之后，运维需要一条「正文还读得出来吗」
// 的快速证据。抽样是**确定性**的：同一份库、同样的抽样数跑两次得到同一批章节，
// 便于前后对比与复现。
type ContentCheckReport struct {
	TotalChapters int   // 库里章节总数
	Sampled       int   // 实际校验的章节数
	EmptyChapters int   // 校验到的空章数（空章合法，但值得报出来）
	MinBytes      int   // 校验章节里的最小字节数
	MaxBytes      int   // 校验章节里的最大字节数
	TotalBytes    int64 // 校验章节中**成功解压**的正文总字节（读不出的章节不计入）
	Blocks        int   // 触及的不同压缩块数

	// BadUTF8 是正文不是合法 UTF-8 的章节数。
	//
	// ⚠️ 这是**内容质量**问题，不是存储完整性故障，因此**不构成失败条件**。
	//
	// 来源几乎总是源 txt 本身的编码问题（例如某个文件主体是 GBK、夹了一段 UTF-8），
	// 解析器按整文件探测编码时只能挑一个，混编段落就会留下非法字节。迁移是逐字节
	// 搬运的，这类脏数据会被原样保留——把它报成「迁移出错」既冤枉又误导排障方向。
	//
	// 真正的存储故障判据只有一条：**解压出来的字节 != 当初写进去的字节**。
	// 所以 FAIL 只由 Corrupted（解压失败/越界/块缺失）决定。
	BadUTF8 int

	// BadUTF8ChapterIDs 记录抽查到的不合法章节 id（最多保留前若干条，供 --show-bad 用）。
	BadUTF8ChapterIDs []int64
	// FailedChapters 是解压/切片失败、因而读不出正文的章节数。
	// Sampled 是「尝试校验的章节数」，两者之差就是成功读出的章数：
	// 不把这两个数分开，运维会疑惑「明明校验了 N 章，正文总字节怎么少了」。
	FailedChapters int

	// Corrupted 收集**全部**损坏块，而不是遇到第一个就停。
	//
	// 几万本书的库里可能同时有多处磁盘损坏；只报第一个会让运维修完一处再跑一次、
	// 反复往返。这里一次列全，配合 Errors() 输出成清单。
	Corrupted []*BlockError
}

// Errors 返回损坏块的简要清单（每块一行，含 book_id / seq / 章节范围）。
func (r ContentCheckReport) Errors() []string {
	out := make([]string, 0, len(r.Corrupted))
	for _, be := range r.Corrupted {
		out = append(out, be.Error())
	}
	return out
}

// defaultContentSample 是 --sample 缺省时的抽样数。
const defaultContentSample = 50

// maxReportedBadUTF8IDs 是报告里最多保留的「非法 UTF-8 章节 id」条数。
//
// 只是给运维一个可追查的线索（去源文件里对照），不是完整清单——真实库可能有
// 成百上千章受同一段混编污染，全列进输出会把终端刷爆。
const maxReportedBadUTF8IDs = 20

// ContentCheckOptions 控制一次自查的范围。
//
// All 与 Sample 互斥（由调用方/CLI 保证）：All 为 true 时忽略 Sample。
type ContentCheckOptions struct {
	All    bool // 全量校验所有章节
	Sample int  // 抽查章节数（<=0 时用 defaultContentSample）
}

// CheckContent 按选项抽样/全量解压章节并校验正文可读。
//
// ## 判据：只认「存储完整性」，不认「内容质量」
//
// PASS 的条件（三条，缺一即失败）：
//  1. 块能解压；
//  2. 该章的 block_off/block_len 落在块内、不越界；
//  3. 能按区间切出字节。
//
// 这三条成立就意味着「读回来的字节 == 当初写进去的字节」，存储是完好的。
//
// **正文不是合法 UTF-8 不算失败**：那是源文件的编码问题（见 BadUTF8 字段注释），
// 和迁移/存储无关。它只作为一个**提示**统计出来，不影响退出码——否则用户迁移完
// 看到 FAIL 会以为数据坏了，去查一个根本不存在的存储故障。
//
// ## 返回值约定
//
// error 只在「连统计都做不下去」时出现（例如 chapters 表读不了）。
// 「有块损坏」通过 report.Corrupted 表达：调用方既能拿完整清单，
// 又能自己决定怎么呈现；CLI 据有无损坏决定退出码。
func (s *Store) CheckContent(ctx context.Context, opt ContentCheckOptions) (ContentCheckReport, error) {
	var rep ContentCheckReport
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters").Scan(&rep.TotalChapters); err != nil {
		return rep, fmt.Errorf("store: check content count: %w", err)
	}
	if rep.TotalChapters == 0 {
		return rep, nil
	}

	target := normalizeSample(opt, rep.TotalChapters)
	ids, err := s.sampleChapterIDs(ctx, target)
	if err != nil {
		return rep, err
	}
	acc := newContentAccumulator(len(ids))
	// 同一个块只解压一次：全量校验时 1 块约 35 章，重复解压会白白多花 35 倍时间。
	cache := make(map[int64][]byte, len(ids)/16+1)
	for _, id := range ids {
		s.checkOneChapter(ctx, id, acc, cache)
	}
	acc.finish()
	acc.rep.TotalChapters = rep.TotalChapters
	return acc.rep, nil
}

// normalizeSample 把选项收敛成「实际要校验的章节数」。
func normalizeSample(opt ContentCheckOptions, total int) int {
	if opt.All {
		return total
	}
	n := opt.Sample
	if n <= 0 {
		n = defaultContentSample
	}
	if n > total {
		n = total
	}
	return n
}

// contentAccumulator 逐章累加自查统计。
type contentAccumulator struct {
	rep       ContentCheckReport
	blocks    map[int64]struct{}
	currentID int64 // 正在校验的章节 id（供 add 记录不合法的具体章节）
}

// newContentAccumulator 造一个带容量预估的累加器。
func newContentAccumulator(size int) *contentAccumulator {
	return &contentAccumulator{
		rep:    ContentCheckReport{MinBytes: -1},
		blocks: make(map[int64]struct{}, size/16+1),
	}
}

// checkOneChapter 校验单章：读块寻址 → 解压（带块内缓存）→ 切片 → 累加统计。
//
// 块损坏只记进 Corrupted 并继续下一章——这是「列出全部坏块」的前提。
func (s *Store) checkOneChapter(ctx context.Context, chapterID int64, acc *contentAccumulator, cache map[int64][]byte) {
	acc.currentID = chapterID
	blockID, off, length, err := s.chapterBlockRef(ctx, chapterID)
	if err != nil {
		acc.addCorrupt(&BlockError{ChapterID: chapterID, Cause: err})
		return
	}
	if blockID <= 0 {
		if length != 0 {
			acc.addCorrupt(&BlockError{ChapterID: chapterID,
				Cause: fmt.Errorf("章节长度为 %d 但没有归属块", length)})
			return
		}
		acc.add("", 0)
		return
	}

	block, ok := cache[blockID]
	if !ok {
		block, err = loadBlock(ctx, s.db, blockID, chapterID)
		if err != nil {
			var be *BlockError
			if ok := asBlockError(err, &be); ok {
				acc.addCorrupt(be)
			} else {
				acc.addCorrupt(&BlockError{BlockID: blockID, ChapterID: chapterID, Cause: err})
			}
			return
		}
		cache[blockID] = block
	}

	text, err := chapterAt(block, off, length)
	if err != nil {
		acc.addCorrupt(&BlockError{BlockID: blockID, ChapterID: chapterID, Cause: err})
		return
	}
	acc.add(text, blockID)
}

// add 累加一章的成功统计。
func (a *contentAccumulator) add(text string, blockID int64) {
	n := len(text)
	a.rep.Sampled++
	a.rep.TotalBytes += int64(n)
	if n == 0 {
		a.rep.EmptyChapters++
	}
	if a.rep.MinBytes < 0 || n < a.rep.MinBytes {
		a.rep.MinBytes = n
	}
	if n > a.rep.MaxBytes {
		a.rep.MaxBytes = n
	}
	// 非法 UTF-8 只统计、不判失败：它是源数据的编码问题，不是存储损坏。
	if !utf8.ValidString(text) {
		a.rep.BadUTF8++
		if len(a.rep.BadUTF8ChapterIDs) < maxReportedBadUTF8IDs {
			a.rep.BadUTF8ChapterIDs = append(a.rep.BadUTF8ChapterIDs, a.currentID)
		}
	}
	if blockID > 0 {
		a.blocks[blockID] = struct{}{}
	}
}

// addCorrupt 记一个损坏块（同一块只记一次，避免 35 章刷 35 条）。
//
// FailedChapters 每章都累加（因为是按章尝试的），Corrupted 按块去重
// （同一个块覆盖的 35 章只报一行，否则清单会长到没法看）。
func (a *contentAccumulator) addCorrupt(be *BlockError) {
	a.rep.Sampled++
	a.rep.FailedChapters++
	for _, seen := range a.rep.Corrupted {
		if seen.BlockID == be.BlockID && be.BlockID != 0 {
			return
		}
		if be.BlockID == 0 && seen.ChapterID == be.ChapterID {
			return
		}
	}
	a.rep.Corrupted = append(a.rep.Corrupted, be)
}

// finish 收尾：修正 MinBytes 初值与块计数。
func (a *contentAccumulator) finish() {
	if a.rep.MinBytes < 0 {
		a.rep.MinBytes = 0
	}
	a.rep.Blocks = len(a.blocks)
}

// sampleChapterIDs 按 id 升序等距抽取 n 个章节 id。
func (s *Store) sampleChapterIDs(ctx context.Context, n int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM chapters ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("store: check content ids: %w", err)
	}
	all, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	if n <= 0 || n >= len(all) {
		return all, nil
	}
	step := len(all) / n
	out := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i*step])
	}
	return out, nil
}

// chapterBlockRef 读一章的块寻址三元组。
func (s *Store) chapterBlockRef(ctx context.Context, chapterID int64) (blockID int64, off, length int, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT block_id, block_off, block_len FROM chapters WHERE id = ?", chapterID).
		Scan(&blockID, &off, &length)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: check chapter %d refs: %w", chapterID, err)
	}
	return blockID, off, length, nil
}

// asBlockError 是 errors.As 的薄封装。
func asBlockError(err error, target **BlockError) bool {
	return errors.As(err, target)
}

// BookContentDigest 返回一本书全部正文的聚合指纹（确定性、与存储形态无关）。
//
// 迁移前后各算一次并比对，就是「这本书的正文一个字节都没变」的直接证据。
// 按 idx/id 排序后依次喂入 FNV-1a：顺序变化也会反映在指纹里。
func (s *Store) BookContentDigest(ctx context.Context, bookID int64) (uint64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return 0, fmt.Errorf("store: digest chapter ids: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return 0, err
	}
	h := fnv.New64a()
	cache := make(map[int64][]byte, 4)
	for _, id := range ids {
		text := s.digestChapter(ctx, id, cache, h)
		if text < 0 {
			return 0, fmt.Errorf("store: book %d 正文读取失败（见上文块定位）", bookID)
		}
	}
	return h.Sum64(), nil
}

// digestChapter 把一章正文喂进哈希，返回 -1 表示失败（已由 loadBlock 记录定位信息）。
func (s *Store) digestChapter(ctx context.Context, chapterID int64, cache map[int64][]byte, h interface{ Write([]byte) (int, error) }) int {
	blockID, off, length, err := s.chapterBlockRef(ctx, chapterID)
	if err != nil {
		return -1
	}
	var text string
	if blockID > 0 {
		block, ok := cache[blockID]
		if !ok {
			block, err = loadBlock(ctx, s.db, blockID, chapterID)
			if err != nil {
				return -1
			}
			cache[blockID] = block
		}
		text, err = chapterAt(block, off, length)
		if err != nil {
			return -1
		}
	}
	_, _ = h.Write([]byte(text))
	_, _ = h.Write([]byte{0}) // 章节分隔符，避免「上一章结尾+下一章开头」拼出新串
	return 0
}

// scanIDs 把「单列 id」的结果集读成切片并关闭。
func scanIDs(rows *sql.Rows) ([]int64, error) {
	defer rows.Close()
	ids := make([]int64, 0, 128)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ids: %w", err)
	}
	return ids, nil
}
