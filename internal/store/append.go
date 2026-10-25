package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"norvels/internal/model"
)

// 本文件是「追更」写入：新章节序列必须是旧序列的前缀，只追加尾部章节。
//
// 为什么不能整本重写：reading_progress.chapter_id 没有外键（0003），整本重建会让
// 所有章节 id 变化，读者的进度记录随即悬空。只有「旧章节一行不动、只往后加」才能
// 既追更又保住进度。因此这里不做任何模糊匹配：对不上就直接报 ErrNotAppendable。

// appendBookInTx 把 b 相对库中已有章节多出来的尾部章节追加进书，返回追加章数。
//
// 调用方（SaveBook）已确认 bookID 有效且 opt.Update 为真。返回值就是 tail 长度：
// 追更角标（books.new_chapters）的累加口径只有这里算过一次，上层直接拿去用。
func appendBookInTx(ctx context.Context, tx *sql.Tx, bookID int64, b *model.ParsedBook, w bookWrite) (int, error) {
	oldTitles, err := loadChapterTitlesInTx(ctx, tx, bookID)
	if err != nil {
		return 0, err
	}
	if err := checkAppendable(oldTitles, b.Chapters); err != nil {
		return 0, err
	}
	appended := 0
	if tail := b.Chapters[len(oldTitles):]; len(tail) > 0 {
		if err := appendChaptersInTx(ctx, tx, bookID, b, tail); err != nil {
			return 0, err
		}
		appended = len(tail)
	}
	if err := updateBookAfterAppend(ctx, tx, bookID, w); err != nil {
		return 0, err
	}
	return appended, nil
}

// loadChapterTitlesInTx 按目录顺序读已有章节标题（排序与 GetBook 一致）。
func loadChapterTitlesInTx(ctx context.Context, tx *sql.Tx, bookID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT title FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: load chapter titles %d: %w", bookID, err)
	}
	defer rows.Close()

	titles := make([]string, 0, 64)
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, fmt.Errorf("store: scan chapter title: %w", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: load chapter titles rows: %w", err)
	}
	return titles, nil
}

// checkAppendable 校验新章节序列以旧序列为前缀；不成立时返回 ErrNotAppendable。
//
// 比较口径：两侧都 TrimSpace 后**整串全等**。不比章号、不做模糊匹配、不把
// 「第1章」与「第一章」当同一章——这类改写无法用字符串判定，猜错的代价是覆盖。
// 章节变少同样拒绝：要么文件被截断，要么这本就不是同一本书，两种都不该截断库里的书。
func checkAppendable(oldTitles []string, chapters []model.ParsedChapter) error {
	if len(chapters) < len(oldTitles) {
		return fmt.Errorf("%w: 新文件 %d 章，少于库中已有的 %d 章（拒绝截断）",
			ErrNotAppendable, len(chapters), len(oldTitles))
	}
	for i, oldTitle := range oldTitles {
		if strings.TrimSpace(oldTitle) != strings.TrimSpace(chapters[i].Title) {
			return fmt.Errorf("%w: 第 %d 章标题 %q 与库中 %q 不一致",
				ErrNotAppendable, i+1, chapters[i].Title, oldTitle)
		}
	}
	return nil
}

// appendChaptersInTx 写入尾部章节的压缩块与章节行，并累加 metrics。
//
// 块序号从已有块的 MAX(seq) 续写（chapter_blocks 上有 UNIQUE(book_id, seq)），
// 章节 idx 直接用解析结果（前缀相同，尾部章节的 idx 天然接续）。
func appendChaptersInTx(ctx context.Context, tx *sql.Tx, bookID int64,
	b *model.ParsedBook, tail []model.ParsedChapter) error {
	volMap, err := appendVolumeMap(ctx, tx, bookID, b, tail)
	if err != nil {
		return err
	}
	startSeq, err := maxBlockSeqInTx(ctx, tx, bookID)
	if err != nil {
		return err
	}
	sizes := make([]int, len(tail))
	for i := range tail {
		sizes[i] = len(tail[i].Content)
	}
	refs, packedBytes, rawBytes, err := writeChapterBlocks(ctx, tx, bookID, tail, sizes, startSeq)
	if err != nil {
		return err
	}
	words, err := insertChapterRows(ctx, tx, bookID, tail, volMap, refs)
	if err != nil {
		return err
	}
	if err := bumpMetrics(ctx, tx, packedBytes, words); err != nil {
		return err
	}
	if err := bumpMetricKey(ctx, tx, metricTextBytesRaw, rawBytes); err != nil {
		return err
	}
	_, err = bumpBookTextBytes(ctx, tx, bookID)
	return err
}

// appendVolumeMap 为尾部章节准备「解析侧卷序号 → 库内卷 id」映射。
//
// 已有同 idx 的卷直接复用：追更时整份文件会被重新解析，卷也重新产出，不复用就会
// 撞 volumes 上的 UNIQUE(book_id, idx)。只有真正新增的卷才建行。
func appendVolumeMap(ctx context.Context, tx *sql.Tx, bookID int64,
	b *model.ParsedBook, tail []model.ParsedChapter) (map[int]int64, error) {
	existing, err := existingVolumeIDs(ctx, tx, bookID)
	if err != nil {
		return nil, err
	}
	volMap := make(map[int]int64, len(existing))
	for vi := range b.Volumes {
		if id, ok := existing[b.Volumes[vi].Idx]; ok {
			volMap[vi] = id
		}
	}
	for i := range tail {
		vi := tail[i].VolumeIdx
		if vi < 0 || vi >= len(b.Volumes) {
			continue
		}
		if _, ok := volMap[vi]; ok {
			continue
		}
		id, err := insertVolumeRow(ctx, tx, bookID, b.Volumes[vi])
		if err != nil {
			return nil, err
		}
		volMap[vi] = id
	}
	return volMap, nil
}

// existingVolumeIDs 返回库里已有的「卷 idx → 卷 id」映射。
func existingVolumeIDs(ctx context.Context, tx *sql.Tx, bookID int64) (map[int]int64, error) {
	rows, err := tx.QueryContext(ctx, "SELECT idx, id FROM volumes WHERE book_id = ?", bookID)
	if err != nil {
		return nil, fmt.Errorf("store: load volume ids %d: %w", bookID, err)
	}
	defer rows.Close()

	out := make(map[int]int64, 8)
	for rows.Next() {
		var idx int
		var id int64
		if err := rows.Scan(&idx, &id); err != nil {
			return nil, fmt.Errorf("store: scan volume id: %w", err)
		}
		out[idx] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: load volume ids rows: %w", err)
	}
	return out, nil
}

// maxBlockSeqInTx 返回已有块的最大 seq（0 表示还没有块），追更续写从它往下排。
func maxBlockSeqInTx(ctx context.Context, tx *sql.Tx, bookID int64) (int, error) {
	var seq int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq), 0) FROM chapter_blocks WHERE book_id = ?", bookID).Scan(&seq); err != nil {
		return 0, fmt.Errorf("store: max block seq %d: %w", bookID, err)
	}
	return seq, nil
}

// updateBookAfterAppend 刷新书级计数：章节数/字数按新文件的全量口径写回。
//
// 三处故意不写：
//   - imported_at 是首次入库时间，追更不重置；
//   - source_path / source_name 仍是同一本书的路径；
//   - books_authors 不重写——读者可能手工改过多作者，追更不该把它冲掉。
//
// file_sha256 有 UNIQUE 约束：新哈希若已被另一本书占用（用户先单独导过更新版），
// 保留旧哈希、只更新计数，避免把一次合法的追更拖成约束错误。
func updateBookAfterAppend(ctx context.Context, tx *sql.Tx, bookID int64, w bookWrite) error {
	sha := w.sha
	taken, err := shaTakenByOther(ctx, tx, w.sha, bookID)
	if err != nil {
		return err
	}
	if taken {
		sha = ""
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE books SET chapter_count = ?, word_count = ?, file_size = ?, "+
			"file_sha256 = CASE WHEN ? = '' THEN file_sha256 ELSE ? END, "+
			"volume_count = (SELECT COUNT(*) FROM volumes WHERE book_id = books.id), updated_at = ? "+
			"WHERE id = ?",
		w.chapterCount, w.wordCount, w.fileSize, sha, sha, w.nowText, bookID)
	if err != nil {
		return fmt.Errorf("store: update book after append %d: %w", bookID, err)
	}
	return nil
}

// shaTakenByOther 判断这个文件哈希是否已被另一本书占用。
func shaTakenByOther(ctx context.Context, tx *sql.Tx, sha string, bookID int64) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM books WHERE file_sha256 = ? AND id <> ?)", sha, bookID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: check sha owner: %w", err)
	}
	return exists, nil
}
