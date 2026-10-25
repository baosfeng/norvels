package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// 章节标题重编号（管理页 M1）。
//
// 只改 title：不动 block_id/block_off/block_len，也不动 idx。理由——「第 N 章」的 N
// 就是 idx+1，序号本身由目录顺序决定；重编号只是把标题文字对齐到现有序号上。
// 因此这是纯文本列的 UPDATE，不需要解压任何块，几万章的书也是毫秒级。

// RenumberScope 指定重编号范围（零值 = 全书）。
//
// 选 volumes.id 而不是「卷序号 volumes.idx」：卷序号在增删卷时会整体漂移，
// 按序号传值容易改错卷；调用方从 GetBook / ListChapters 拿到的 VolumeID 就是
// volumes.id，原样传回来不会错位。
// chapters.volume_id = 0 表示「未分卷」，所以 0 只能当「全书」，
// 「只改未分卷的章」不在本接口能力内（那种需求应该先建卷）。
type RenumberScope struct {
	VolumeID int64 // > 0：只重编号这一卷内的章节；0：全书
}

// RenumberChapters 按 idx+1 重写标题：先剥掉原标题里已有的章号前缀，再拼「第 N 章」。
//
// 序号 N 是**全书**序号（idx+1），单卷范围下也不从 1 重来：目录里显示的章号与 idx
// 必须一一对应，否则「第 3 章」在全书里会有两个。
func (s *Store) RenumberChapters(ctx context.Context, bookID int64, scope RenumberScope) error {
	if bookID <= 0 || scope.VolumeID < 0 {
		return fmt.Errorf("%w: book %d volume %d", ErrInvalidArgument, bookID, scope.VolumeID)
	}
	return s.inTx(ctx, "renumber chapters", func(tx *sql.Tx) error {
		targets, err := renumberTargets(ctx, tx, bookID, scope)
		if err != nil {
			return err
		}
		for _, t := range targets {
			if err := writeRenumberedTitle(ctx, tx, bookID, t); err != nil {
				return err
			}
		}
		// 标题变了，书的「最后更新」也该动（详情页与列表都按 updated_at 排序）；
		// 书不存在时这里返回 ErrNotFound，避免「改 0 章却报成功」。
		return touchBookUpdatedAt(ctx, tx, bookID)
	})
}

// renumberTarget 是一行待重编号的章节（只取重编号需要的三列）。
type renumberTarget struct {
	id    int64
	idx   int
	title string
}

// renumberTargets 读出范围内的章节（按目录顺序）。
func renumberTargets(ctx context.Context, tx *sql.Tx, bookID int64, scope RenumberScope) ([]renumberTarget, error) {
	q := "SELECT id, idx, title FROM chapters WHERE book_id = ?"
	args := []any{bookID}
	if scope.VolumeID > 0 {
		// 卷 id 可能来自别的书：不校验就会静默「改 0 章」并返回成功。
		if err := requireVolumeOfBook(ctx, tx, bookID, scope.VolumeID); err != nil {
			return nil, err
		}
		q += " AND volume_id = ?"
		args = append(args, scope.VolumeID)
	}
	rows, err := tx.QueryContext(ctx, q+" ORDER BY idx, id", args...)
	if err != nil {
		return nil, fmt.Errorf("store: list renumber targets %d: %w", bookID, err)
	}
	defer rows.Close()

	out := make([]renumberTarget, 0, 128)
	for rows.Next() {
		var t renumberTarget
		if err := rows.Scan(&t.id, &t.idx, &t.title); err != nil {
			return nil, fmt.Errorf("store: scan renumber target: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list renumber targets rows %d: %w", bookID, err)
	}
	return out, nil
}

// requireVolumeOfBook 校验卷属于这本书；不属于（或卷已删）返回 ErrNotFound。
func requireVolumeOfBook(ctx context.Context, tx *sql.Tx, bookID, volumeID int64) error {
	var found int64
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM volumes WHERE id = ? AND book_id = ?", volumeID, bookID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: volume %d of book %d", ErrNotFound, volumeID, bookID)
	}
	if err != nil {
		return fmt.Errorf("store: volume lookup %d: %w", volumeID, err)
	}
	return nil
}

// writeRenumberedTitle 写回一章的新标题；算出来跟原标题一样就不写（省掉无意义的行更新）。
func writeRenumberedTitle(ctx context.Context, tx *sql.Tx, bookID int64, t renumberTarget) error {
	title := renumberedTitle(t.idx, t.title)
	if title == t.title {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE chapters SET title = ? WHERE id = ? AND book_id = ?", title, t.id, bookID); err != nil {
		return fmt.Errorf("store: renumber chapter %d: %w", t.id, err)
	}
	return nil
}

// renumberedTitle 生成「第 N 章」+ 原标题里章号之后的部分；余下为空时只留「第 N 章」。
func renumberedTitle(idx int, title string) string {
	prefix := fmt.Sprintf("第 %d 章", idx+1)
	rest := stripChapterNumberPrefix(title)
	if rest == "" {
		return prefix
	}
	return prefix + " " + rest
}

// chapterNumeralRunes 是章号里可能出现的中文数字（「第十二章」「第三回」）。
const chapterNumeralRunes = "零〇一二三四五六七八九十百千万两"

// chapterNumberSuffixRunes 是章号后允许跟的量词（「第 3 节」「第12回」）。
var chapterNumberSuffixRunes = [...]rune{'章', '回', '节'}

// stripChapterNumberPrefix 去掉原标题里已有的章号前缀，返回章号之后的标题文字。
//
// 为什么要剥：书里常写「第十二章 风起云涌」，重编号后再拼一次会变成
// 「第 13 章 第十二章 风起云涌」。剥的范围只到量词与其后的分隔符；不认识的形态
// （「卷三 风起」「楔子」「第一次见面」）原样保留——猜错比不改更糟。
func stripChapterNumberPrefix(title string) string {
	trimmed := strings.TrimSpace(title)
	r := []rune(trimmed)
	after, ok := chapterNumberPrefixEnd(r)
	if !ok {
		return trimmed
	}
	return strings.TrimSpace(string(r[skipSeparators(r, after):]))
}

// chapterNumberPrefixEnd 返回「第 N 章/回/节」这段章号之后的 rune 下标；
// 不是章号前缀时 ok=false（由调用方原样保留标题）。
func chapterNumberPrefixEnd(r []rune) (int, bool) {
	if len(r) < 3 || r[0] != '第' {
		return 0, false
	}
	// 「第 12 章」里 第 与数字之间、数字与量词之间都可能有空格。
	numStart := skipSeparators(r, 1)
	numEnd := skipNumerals(r, numStart)
	if numEnd == numStart {
		return 0, false
	}
	suffix := skipSeparators(r, numEnd)
	if suffix >= len(r) || !isChapterNumberSuffix(r[suffix]) {
		return 0, false
	}
	return suffix + 1, true
}

// skipSeparators 跳过从 i 开始的一段标题分隔符。
func skipSeparators(r []rune, i int) int {
	for i < len(r) && isChapterTitleSeparator(r[i]) {
		i++
	}
	return i
}

// skipNumerals 跳过从 i 开始的一段章号数字（中文数字或阿拉伯数字）。
func skipNumerals(r []rune, i int) int {
	for i < len(r) && isChapterNumeral(r[i]) {
		i++
	}
	return i
}

// isChapterNumeral 判断是否算章号里的数字（阿拉伯数字含全角，或中文数字）。
func isChapterNumeral(r rune) bool {
	return unicode.IsDigit(r) || strings.ContainsRune(chapterNumeralRunes, r)
}

// isChapterNumberSuffix 判断是否章号后的量词。
func isChapterNumberSuffix(r rune) bool {
	for _, s := range chapterNumberSuffixRunes {
		if r == s {
			return true
		}
	}
	return false
}

// isChapterTitleSeparator 判断章号与标题之间的分隔符（含空格与全角标点）。
func isChapterTitleSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\u3000', '\u00a0', '、', '．', '.', '：', ':', '·', '，', ',':
		return true
	}
	return false
}
