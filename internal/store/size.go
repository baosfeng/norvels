package store

import (
	"context"
	"database/sql"
)

// 体积构成的读路径口径（必须在毫秒级完成）：
//
//   - 正文类（textBytes）= books 的文本列 + chapters 的正文与字级分词镜像列。
//     分词镜像列 content_seg/title_seg 物理上存在 chapters 表里，会计入「正文」侧，
//     这是刻意的：用户看到的「正文占多少」应当包含为检索而做的副本。
//   - 索引类（indexBytes）= FTS5 影子表字节数（按「库文件大小 − 正文」近似，见 refreshMetrics）。
//   - 其他（otherBytes）= 库文件总大小 − 正文 − 索引，含 btree 页填充、表头、空闲页等。
//
// 精确口径要扫 4.2 GB 正文 + 4.2 GB 镜像列，实测 99 s（chapters）+ 20.9 s（FTS 影子表），
// 放在 /api/stats 请求路径上不可接受。因此这些值由导入时顺手累加进 store_metrics，
// 读接口只查表；缓存缺失或口径落后时接口照常返回，但把 Estimated 标为 true。
const (
	// compositionSampleSize 已废弃：改为预计算，保留常量仅为兼容旧引用。
	compositionSampleSize = "1000"
	// bookTextSQL 是 books 侧文本字节（838 行，全表扫也是毫秒级，不必缓存）。
	bookTextSQL = "SELECT COALESCE(SUM(length(title) + length(intro) + length(source_path) + " +
		"length(source_name) + length(encoding) + length(status)), 0) FROM books"
	// chapterTextSampleSQL 已废弃：预计算口径下不再抽样。
	chapterTextSampleSQL = "SELECT COALESCE(SUM(length(title) + length(content) + length(title_seg) + " +
		"length(content_seg)), 0) FROM chapters WHERE id = -1"
	// chapterCountSQL 只数行数，走 btree 扫描（实测 0.02 s）。
	chapterCountSQL = "SELECT COUNT(*) FROM chapters"
	// ftsDataBytesSQL 是 FTS 内容块总量（慢查询，只在全量重算里用）。
	ftsDataBytesSQL = "SELECT COALESCE(SUM(length(block)), 0) FROM chapters_fts_data"
	// smallIndexBytesSQL 汇总 FTS 的其余影子表（都很小，实测合计 0.15 s）。
	smallIndexBytesSQL = "SELECT " +
		"(SELECT COALESCE(SUM(length(segid)), 0) FROM chapters_fts_idx) + " +
		"(SELECT COALESCE(SUM(length(sz)), 0) FROM chapters_fts_docsize) + " +
		"(SELECT COALESCE(SUM(length(k) + length(v)), 0) FROM chapters_fts_config)"
)

// Composition 是一次体积构成的结果。
//
// Estimated 表示数值来自预计算缓存而非当场全表统计（缓存尚未算好时为 true）。
type Composition struct {
	TextBytes  int64
	IndexBytes int64
	Estimated  bool
}

// Composition 返回体积构成，只查预计算表（毫秒级）。
func (s *Store) Composition(ctx context.Context) (Composition, error) {
	snap, err := s.Metrics(ctx)
	if err != nil {
		return Composition{}, err
	}
	return Composition{
		TextBytes:  snap.TextBytes,
		IndexBytes: snap.IndexBytes,
		Estimated:  snap.Stale,
	}, nil
}

// scalarInt64 执行单值查询（NULL 按 0）。
func (s *Store) scalarInt64(ctx context.Context, query string, args ...any) (int64, error) {
	var value sql.NullInt64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		return 0, err
	}
	return value.Int64, nil
}

// ensureCompositionIndex 幂等创建 chapters(word_count) 覆盖索引（见 0004 迁移注释）。
func (s *Store) ensureCompositionIndex(ctx context.Context) error {
	exists, err := s.tableExists(ctx, "chapters")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		"CREATE INDEX IF NOT EXISTS idx_chapters_wc ON chapters(word_count)"); err != nil {
		return err
	}
	return nil
}

// fillComposition 把估算结果写进 Stats；otherBytes 以 DBBytes 为上限兜底。
//
// 三个分量之和恒等于 DBBytes：正文与索引的估算值可能超过实际文件大小
// （压缩页、空闲页回收等），此时按比例压缩，保证前端堆叠条不出现负值。
func fillComposition(st *Stats, textBytes, indexBytes int64) {
	if st.DBBytes <= 0 {
		st.TextBytes, st.IndexBytes, st.OtherBytes = textBytes, indexBytes, 0
		return
	}
	total := textBytes + indexBytes
	if total <= 0 {
		st.TextBytes, st.IndexBytes, st.OtherBytes = 0, 0, st.DBBytes
		return
	}
	if total > st.DBBytes {
		st.TextBytes = textBytes * st.DBBytes / total
		st.IndexBytes = indexBytes * st.DBBytes / total
		st.OtherBytes = st.DBBytes - st.TextBytes - st.IndexBytes
		return
	}
	st.TextBytes = textBytes
	st.IndexBytes = indexBytes
	st.OtherBytes = st.DBBytes - textBytes - indexBytes
}
