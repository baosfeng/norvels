package store

import (
	"context"
	"fmt"
	"time"
)

// 本文件是「存量书指纹回填」：0019 只建表，历史书的正文没有指纹，
// 新导入的书因此比不到它们（判重半残）。回填逐书解压正文、算章级指纹写回。
//
// 语义（这三条决定了它能不能在生产上反复跑）：
//   - **跳过式**：已经有指纹的书整本跳过。已有指纹 = 这本书的正文块没变过
//     （SaveBook / 追更 / 编辑都会重写指纹），重写只是白烧 CPU；中断后重跑即从断点继续。
//   - **按书原子**：一本书的指纹在一个正文库事务里写完，不存在「半本书有指纹」的状态。
//   - **坏块不拦整批**：解压失败 / 区间越界的章节计入 BadChapters 后跳过，
//     其余书照常回填——回填是运维动作，遇到一本坏书就全停等于让运维无从下手。

// FingerprintBackfillOptions 是一次回填的粒度与回调。
type FingerprintBackfillOptions struct {
	// Batch 是每批处理的书数（0 → defaultBackfillBatch）：只影响进度回调粒度与
	// 每批之间的 ctx 检查频率，不影响事务粒度（事务是按书开的）。
	Batch int
	// Progress 每处理完一批书回调一次（可为 nil）：done 是已扫描书数，total 是主库总书数。
	Progress func(done, total int, elapsed time.Duration)
}

// FingerprintBackfillReport 是一次回填的汇总（CLI 直接打印，运维据此判断要不要重跑）。
type FingerprintBackfillReport struct {
	Books        int   // 扫描过的书数
	Filled       int   // 本次写入指纹的书数
	Skipped      int   // 已有指纹、整本跳过的书数
	NoContent    int   // 解压后写不出任何指纹的书数（正文全空或全是坏块）
	Fingerprints int64 // 写入的指纹行数
	BadChapters  int   // 坏章节数（块缺失 / 解压失败 / 区间越界）
	Elapsed      time.Duration
}

// defaultBackfillBatch 是默认的进度回调粒度（每 200 本书打一行）。
const defaultBackfillBatch = 200

// BackfillFingerprints 给存量书补写章级指纹（跳过已有指纹的书，可中断续跑）。
//
// 正文库还没有指纹表时直接报错：那是「迁移没跑」，不是「没有可回填的书」，
// 让运维看到「先跑 norvels migrate」比让他以为回填成功了要好。
func (s *Store) BackfillFingerprints(ctx context.Context, opt FingerprintBackfillOptions) (FingerprintBackfillReport, error) {
	start := time.Now()
	var rep FingerprintBackfillReport
	plan, err := s.backfillPlan(ctx, opt.Batch)
	if err != nil {
		return rep, err
	}
	lastID := int64(0)
	for {
		ids, err := s.bookIDsAfter(ctx, lastID, plan.batch)
		if err != nil {
			return rep, err
		}
		if len(ids) == 0 {
			break
		}
		lastID = ids[len(ids)-1]
		if err := s.backfillBatch(ctx, ids, plan.done, &rep); err != nil {
			rep.Elapsed = time.Since(start)
			return rep, err
		}
		reportBackfillProgress(opt.Progress, rep.Books, plan.total, start)
	}
	rep.Elapsed = time.Since(start)
	return rep, nil
}

// backfillPlan 是回填前的准备工作：跳过集（已有指纹的书）、总书数与批大小。
//
// 单独拆出来是为了让主循环只管「分批扫描」：准备阶段的四条失败路径（表缺失 / 查跳过集 /
// 数书数 / 批大小归一）都是「告诉运维哪里坏了」，混在循环里会把主流程读成错误清单。
type backfillPlan struct {
	done  map[int64]struct{} // 已有指纹的书 id（整本跳过）
	total int                // 主库总书数（进度分母）
	batch int                // 每批书数
}

// backfillPlan 建一份回填计划；正文库没有指纹表时直接报错（那是「迁移没跑」）。
func (s *Store) backfillPlan(ctx context.Context, batch int) (backfillPlan, error) {
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil {
		return backfillPlan{}, err
	}
	if !ready {
		return backfillPlan{}, fmt.Errorf("store: 正文库没有 chapter_fingerprints（先跑 norvels migrate 应用 0019）")
	}
	done, err := s.fingerprintedBookIDs(ctx)
	if err != nil {
		return backfillPlan{}, err
	}
	total, err := s.bookCount(ctx)
	if err != nil {
		return backfillPlan{}, err
	}
	if batch <= 0 {
		batch = defaultBackfillBatch
	}
	return backfillPlan{done: done, total: total, batch: batch}, nil
}

// reportBackfillProgress 打一批进度（回调为 nil 时什么都不做）。
//
// 两条回填命令（指纹 / 质量分）共用：进度只关心「扫了几本 / 共几本 / 用了多久」。
func reportBackfillProgress(progress func(done, total int, elapsed time.Duration),
	done, total int, start time.Time) {
	if progress != nil {
		progress(done, total, time.Since(start))
	}
}

// backfillBatch 处理一批书的 id（ctx 取消时返回 ctx.Err()，已写入的指纹保留）。
func (s *Store) backfillBatch(ctx context.Context, ids []int64, done map[int64]struct{}, rep *FingerprintBackfillReport) error {
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err //nolint:wrapcheck // 取消原样上抛，CLI 按「中断可续跑」提示
		}
		rep.Books++
		if _, ok := done[id]; ok {
			rep.Skipped++
			continue
		}
		wrote, err := s.backfillBookFingerprints(ctx, id, rep)
		if err != nil {
			return fmt.Errorf("store: 回填书 %d 的指纹: %w", id, err)
		}
		if wrote {
			rep.Filled++
		} else {
			rep.NoContent++
		}
	}
	return nil
}

// backfillBookFingerprints 回填一本书，返回是否写出了指纹。
//
// 按块读：一个块覆盖约 35 章，解压一次就够——逐章调 GetChapter 会把同一块解压几十次。
// 单本书的内存占用因此是「一块原文（≤8 MB）+ 这本书的指纹列表」。
func (s *Store) backfillBookFingerprints(ctx context.Context, bookID int64, rep *FingerprintBackfillReport) (bool, error) {
	refs, err := s.bookChapterRefs(ctx, bookID)
	if err != nil {
		return false, err
	}
	q, err := s.blockDB(ctx)
	if err != nil {
		return false, err
	}
	fps := make([]chapterFP, 0, len(refs))
	for start := 0; start < len(refs); {
		end := blockGroupEnd(refs, start)
		block, err := loadBlock(ctx, q, refs[start].blockID, refs[start].id)
		if err != nil {
			rep.BadChapters += end - start // 整块坏：这块覆盖的章节全算坏，跳过不中断
			start = end
			continue
		}
		fps = appendBlockFingerprints(fps, block, refs[start:end], rep)
		start = end
	}
	if len(fps) == 0 {
		return false, nil
	}
	if err := s.replaceFingerprints(ctx, bookID, fps); err != nil {
		return false, err
	}
	rep.Fingerprints += int64(len(fps))
	return true, nil
}

// blockGroupEnd 返回 refs[start] 所在块的最后一章下标（不含），refs 按章节顺序排列。
func blockGroupEnd(refs []chapterRef, start int) int {
	end := start
	for end < len(refs) && refs[end].blockID == refs[start].blockID {
		end++
	}
	return end
}

// appendBlockFingerprints 从已解压的块里切出这一组章节并算指纹。
func appendBlockFingerprints(dst []chapterFP, block []byte, refs []chapterRef, rep *FingerprintBackfillReport) []chapterFP {
	for _, ref := range refs {
		content, err := chapterAt(block, ref.off, ref.length)
		if err != nil {
			rep.BadChapters++
			continue
		}
		hash, bytes, ok := chapterFingerprint(content)
		if !ok {
			continue // 空白章不参与判重（见 chapterFingerprints）
		}
		dst = append(dst, chapterFP{hash: hash, bytes: bytes})
	}
	return dst
}
