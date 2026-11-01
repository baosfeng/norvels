package store

import (
	"context"
	"fmt"
	"time"
)

// 本文件是「存量书质量分回填」：0020 只加列，历史书的质量分是空的，
// 而版本择优要靠它回答「哪一版更好」（设计 §5）。回填逐书解压正文、重算质量分写回。
//
// 语义（这三条决定了它能不能在生产上反复跑）：
//   - **跳过式**：已经有质量分的书整本跳过。已有质量分 = 这本书的正文没被覆盖重导过
//     （SaveBook 覆盖/追更会重写），重算只是白烧 CPU；中断后重跑即从断点继续。
//   - **按书原子**：一本书的七列在一条 UPDATE 里写完，不存在「半本书有质量分」的状态。
//   - **坏块不拦整批**：解压失败 / 区间越界的章节计入 BadChapters 后跳过，
//     其余书照常回填——回填是运维动作，遇到一本坏书就全停等于让运维无从下手。
//
// 与 backfill-fingerprints 分开成两条命令（而不是合成一条）：指纹回填只算哈希，
// 几分钟到几十分钟；质量回填要把正文解压后逐段统计（还要在全书范围内判内部重复），
// 慢一个量级，运维需要能单独重跑、单独看进度。

// QualityBackfillOptions 是一次回填的粒度与回调。
type QualityBackfillOptions struct {
	// Batch 是每批处理的书数（0 → defaultBackfillBatch）：只影响进度回调粒度与
	// 每批之间的 ctx 检查频率，不影响写入粒度（每本书一条 UPDATE）。
	Batch int
	// Progress 每处理完一批书回调一次（可为 nil）：done 是已扫描书数，total 是主库总书数。
	Progress func(done, total int, elapsed time.Duration)
}

// QualityBackfillReport 是一次回填的汇总（CLI 直接打印，运维据此判断要不要重跑）。
type QualityBackfillReport struct {
	Books       int // 扫描过的书数
	Filled      int // 本次写入质量分的书数
	Skipped     int // 已有质量分、整本跳过的书数
	NoContent   int // 正文全空（或全是坏块）写不出质量分的书数
	BadChapters int // 坏章节数（块缺失 / 解压失败 / 区间越界）
	Elapsed     time.Duration
}

// qualityBackfillPlan 是回填前的准备：跳过集（已有质量分的书）、总书数与批大小。
type qualityBackfillPlan struct {
	done  map[int64]struct{}
	total int
	batch int
}

// BackfillQuality 给存量书补算质量分（跳过已有质量分的书，可中断续跑）。
//
// 质量列不存在（迁移 0020 未应用）时直接报错：那是「迁移没跑」，不是「没有可回填的书」，
// 让运维看到「先跑 norvels migrate」比让他以为回填成功了要好。
func (s *Store) BackfillQuality(ctx context.Context, opt QualityBackfillOptions) (QualityBackfillReport, error) {
	start := time.Now()
	var rep QualityBackfillReport
	plan, err := s.qualityBackfillPlan(ctx, opt.Batch)
	if err != nil {
		return rep, err
	}
	lastID := int64(0)
	for {
		ids, err := s.bookIDsAfter(ctx, lastID, plan.batch)
		if err != nil {
			rep.Elapsed = time.Since(start)
			return rep, err
		}
		if len(ids) == 0 {
			break
		}
		lastID = ids[len(ids)-1]
		if err := s.qualityBackfillBatch(ctx, ids, plan.done, &rep); err != nil {
			rep.Elapsed = time.Since(start)
			return rep, err
		}
		reportBackfillProgress(opt.Progress, rep.Books, plan.total, start)
	}
	rep.Elapsed = time.Since(start)
	return rep, nil
}

// qualityBackfillPlan 建一份回填计划；质量列缺失时直接报错（那是迁移没跑）。
func (s *Store) qualityBackfillPlan(ctx context.Context, batch int) (qualityBackfillPlan, error) {
	ready, err := s.qualityColumnsReady(ctx)
	if err != nil {
		return qualityBackfillPlan{}, err
	}
	if !ready {
		return qualityBackfillPlan{}, fmt.Errorf("store: books 还没有质量列（先跑 norvels migrate 应用 0020）")
	}
	done, err := s.qualityComputedBookIDs(ctx)
	if err != nil {
		return qualityBackfillPlan{}, err
	}
	total, err := s.bookCount(ctx)
	if err != nil {
		return qualityBackfillPlan{}, err
	}
	if batch <= 0 {
		batch = defaultBackfillBatch
	}
	return qualityBackfillPlan{done: done, total: total, batch: batch}, nil
}

// qualityColumnsReady 判断 books 有没有 0020 的质量列（按 quality_at 探测）。
func (s *Store) qualityColumnsReady(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('books') WHERE name = 'quality_at'").Scan(&n); err != nil {
		return false, fmt.Errorf("store: check quality columns: %w", err)
	}
	return n > 0, nil
}

// qualityComputedBookIDs 返回已有质量分的书 id 集合（回填按它做「整本跳过」）。
//
// 判据与 BookQuality.Computed() 同口径：章数 > 0 且时间戳非空。一次性读进内存，
// 两万本约 160 KB，比每本书查一次少两万次往返。
func (s *Store) qualityComputedBookIDs(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM books WHERE quality_chapters > 0 AND quality_at <> ''")
	if err != nil {
		return nil, fmt.Errorf("store: list quality books: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// qualityBackfillBatch 处理一批书的 id（ctx 取消时返回 ctx.Err()，已写入的质量分保留）。
func (s *Store) qualityBackfillBatch(ctx context.Context, ids []int64, done map[int64]struct{},
	rep *QualityBackfillReport) error {
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err //nolint:wrapcheck // 取消原样上抛，CLI 按「中断可续跑」提示
		}
		rep.Books++
		if _, ok := done[id]; ok {
			rep.Skipped++
			continue
		}
		wrote, err := s.backfillBookQuality(ctx, id, rep)
		if err != nil {
			return fmt.Errorf("store: 回填书 %d 的质量分: %w", id, err)
		}
		if wrote {
			rep.Filled++
		} else {
			rep.NoContent++
		}
	}
	return nil
}

// backfillBookQuality 回填一本书，返回是否写出了质量分。
//
// 按块读：一个块覆盖约 35 章，解压一次就够——逐章调 GetChapter 会把同一块解压几十次。
// 单本书的内存占用因此是「一块原文（<=8 MB）+ 章长列表 + 段落哈希集」。
func (s *Store) backfillBookQuality(ctx context.Context, bookID int64, rep *QualityBackfillReport) (bool, error) {
	refs, err := s.bookChapterRefs(ctx, bookID)
	if err != nil {
		return false, err
	}
	q, err := s.blockDB(ctx)
	if err != nil {
		return false, err
	}
	b := newStoredQualityBuilder(len(refs))
	for start := 0; start < len(refs); {
		end := blockGroupEnd(refs, start)
		block, err := loadBlock(ctx, q, refs[start].blockID, refs[start].id)
		if err != nil {
			rep.BadChapters += end - start // 整块坏：这块覆盖的章节全算坏，跳过不中断
			start = end
			continue
		}
		appendBlockContents(b, block, refs[start:end], rep)
		start = end
	}
	quality, ok := b.finish()
	if !ok {
		return false, nil
	}
	return true, s.writeBookQuality(ctx, bookID, quality)
}

// appendBlockContents 从已解压的块里切出这一组章节，喂给质量统计。
func appendBlockContents(b *storedQualityBuilder, block []byte, refs []chapterRef, rep *QualityBackfillReport) {
	for _, ref := range refs {
		content, err := chapterAt(block, ref.off, ref.length)
		if err != nil {
			rep.BadChapters++
			continue
		}
		b.addChapter(content)
	}
}

// writeBookQuality 把质量分写回 books 的七列（一条 UPDATE，按书原子）。
func (s *Store) writeBookQuality(ctx context.Context, bookID int64, q BookQuality) error {
	args := append(q.qualityArgs(), bookID)
	if _, err := s.db.ExecContext(ctx,
		"UPDATE books SET "+bookQualityUpdateAssignments+" WHERE id = ?", args...); err != nil {
		return fmt.Errorf("store: write book quality %d: %w", bookID, err)
	}
	return nil
}
