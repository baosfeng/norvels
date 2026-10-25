package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"
)

// 按段落边界拆分章节（管理页 M1）。
//
// 场景：导入时某几万字仍没被规则切开（缺章节标记、整卷内容挤在一章）。
// 拆分只做一件事——把一个 span 切成几段，正文一个字节都不改：
//
//	原来：[off, off+len) 一整段
//	拆后：[off, off+c1) [off+c1, off+c2) … 各是一章，仍指向同一个压缩块
//
// 所以拆分是 O(拆分点) 的写操作：不解压重压、不搬数据，几万字的章也是毫秒级。

// SplitChapter 按段落边界把一章拆成多章，返回拆完后的章数。
//
// cutAfter 是 0 起的段落下标，含义是「在这一段之后切开」，必须严格递增、
// 落在 [0, 段落数-2]；切出来的每一章都必须有字（空行之间不算切点）。
// 新章标题沿用「原标题（i/N）」，序号紧跟原章，其后各章整体后移。
func (s *Store) SplitChapter(ctx context.Context, bookID, chapterID int64, cutAfter []int) (int, error) {
	if err := checkIDs(bookID, chapterID); err != nil {
		return 0, err
	}
	if len(cutAfter) == 0 {
		return 0, fmt.Errorf("%w: 至少要给一个拆分点", ErrInvalidArgument)
	}
	if len(cutAfter)+1 > maxSplitPieces {
		return 0, fmt.Errorf("%w: 一次最多拆成 %d 章", ErrInvalidArgument, maxSplitPieces)
	}
	pieces := 0
	err := s.inTx(ctx, "split chapter", func(tx *sql.Tx) error {
		sp, err := loadChapterSpan(ctx, tx, bookID, chapterID)
		if err != nil {
			return err
		}
		raw, err := spanText(ctx, tx, sp)
		if err != nil {
			return err
		}
		cuts, err := splitByteOffsets(string(raw), cutAfter)
		if err != nil {
			return err
		}
		pieces = len(cuts) + 1
		return applySplit(ctx, tx, bookID, sp, string(raw), cuts)
	})
	if err != nil {
		return 0, err
	}
	return pieces, nil
}

// applySplit 落实一次拆分：腾位置 → 改写原章 → 插入其余各章 → 计数与块区间。
func applySplit(ctx context.Context, tx *sql.Tx, bookID int64, sp chapterSpan, text string, cuts []int) error {
	pieces := len(cuts) + 1
	titles := splitTitles(sp.title, pieces)
	if err := shiftChaptersIdx(ctx, tx, bookID, sp.idx, pieces-1); err != nil {
		return err
	}
	total, err := rewriteHeadPiece(ctx, tx, sp, text, cuts[0], titles[0])
	if err != nil {
		return err
	}
	if total, err = insertTailPieces(ctx, tx, bookID, sp, text, cuts, titles, total); err != nil {
		return err
	}
	// 字数按各段实际重算，书级计数取差值（不做「拆完必然守恒」的假设：
	// 库里旧 word_count 可能来自解析器口径，与 store.countWords 略有出入）。
	delta := total - sp.wordCount
	if err := adjustBookCounts(ctx, tx, bookID, pieces-1, delta); err != nil {
		return err
	}
	if err := bumpMetrics(ctx, tx, 0, int64(delta)); err != nil {
		return err
	}
	return refreshBlockIdxRange(ctx, tx, sp.blockID)
}

// rewriteHeadPiece 把原章改写为第一段，返回第一段的字数。
func rewriteHeadPiece(ctx context.Context, tx *sql.Tx, sp chapterSpan, text string, cut int, title string) (int, error) {
	words := countWords(text[:cut])
	if _, err := tx.ExecContext(ctx,
		"UPDATE chapters SET title = ?, word_count = ?, block_len = ? WHERE id = ?",
		title, words, cut, sp.id); err != nil {
		return 0, fmt.Errorf("store: split head chapter %d: %w", sp.id, err)
	}
	return words, nil
}

// insertTailPieces 插入第 2..N 段，返回全部段的字数总和。
//
// 它们与原章共享同一个 block_id，只是 block_off/block_len 不同——这正是
// 「一个块覆盖整数章」的自然延伸，读取侧不需要任何改动。
func insertTailPieces(ctx context.Context, tx *sql.Tx, bookID int64, sp chapterSpan, text string,
	cuts []int, titles []string, total int) (int, error) {
	stmt, err := tx.PrepareContext(ctx, chapterInsertSQL)
	if err != nil {
		return 0, fmt.Errorf("store: split chapter prepare: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // 只读句柄

	start := cuts[0]
	for i := 1; i < len(titles); i++ {
		end := len(text)
		if i < len(cuts) {
			end = cuts[i]
		}
		words := countWords(text[start:end])
		idx := sp.idx + i
		if _, err := stmt.ExecContext(ctx, bookID, sp.volumeID, idx, titles[i], words,
			sp.blockID, sp.offset+start, end-start); err != nil {
			return 0, fmt.Errorf("store: split insert chapter idx=%d: %w", idx, err)
		}
		total += words
		start = end
	}
	return total, nil
}

// splitByteOffsets 把段落切点翻译成章内字节偏移，并校验每个切点两侧都有字。
func splitByteOffsets(text string, cutAfter []int) ([]int, error) {
	bounds := lineEndOffsets(text)
	if len(bounds) < 2 {
		return nil, fmt.Errorf("%w: 这一章只有 %d 段，拆不开", ErrInvalidArgument, len(bounds))
	}
	cuts := make([]int, 0, len(cutAfter))
	prev := -1
	for _, idx := range cutAfter {
		if idx <= prev {
			return nil, fmt.Errorf("%w: 拆分点必须严格递增（%d 紧跟在 %d 之后）",
				ErrInvalidArgument, idx, prev)
		}
		if idx < 0 || idx > len(bounds)-2 {
			return nil, fmt.Errorf("%w: 拆分点 %d 超出 0~%d",
				ErrInvalidArgument, idx, len(bounds)-2)
		}
		cuts = append(cuts, bounds[idx])
		prev = idx
	}
	if err := checkPiecesVisible(text, cuts); err != nil {
		return nil, err
	}
	return cuts, nil
}

// checkPiecesVisible 拒绝会产出空章的拆法（切点落在连续空行里）。
func checkPiecesVisible(text string, cuts []int) error {
	ends := append(append([]int{}, cuts...), len(text))
	start := 0
	for _, end := range ends {
		if strings.TrimSpace(text[start:end]) == "" {
			return fmt.Errorf("%w: 拆分点落在了空行上，切出来会是空章", ErrInvalidArgument)
		}
		start = end
	}
	return nil
}

// lineEndOffsets 返回每一段（以换行分隔）结尾后的字节偏移；最后一段没换行也算一段。
func lineEndOffsets(text string) []int {
	out := make([]int, 0, 64)
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, i+1)
		}
	}
	return append(out, len(text))
}

// splitTitles 生成各段标题：单段不改名，多段加「（i/N）」后缀并保证不超长。
func splitTitles(title string, pieces int) []string {
	if pieces <= 1 {
		return []string{title}
	}
	out := make([]string, pieces)
	for i := 0; i < pieces; i++ {
		suffix := fmt.Sprintf("（%d/%d）", i+1, pieces)
		base := truncateRunes(title, maxChapterTitleRunes-utf8.RuneCountInString(suffix))
		out[i] = base + suffix
	}
	return out
}
