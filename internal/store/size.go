package store

import (
	"context"
	"fmt"
)

// 体积构成估算的 SQL：三张口径都只读不写，全部用 COALESCE 把空表算成 0。
//
// 说明：
//   - 正文类（textBytes）= chapters 的正文与字级分词镜像列 + books 的标题/简介/路径等文本列。
//     分词镜像列 content_seg/title_seg 虽然是索引的输入，但物理上存在 chapters 表里，
//     会计入「正文」侧，这是刻意的：用户看到的「正文占多少」应当包含为检索而做的副本。
//   - 索引类（indexBytes）= FTS5 影子表（chapters_fts_data / _idx / _docsize / _config）字节数，
//     即全文索引实际占用的空间。
//   - 其他（otherBytes）= 库文件总大小 − 正文 − 索引，含 btree 页填充、表头、空闲页等。
const (
	textBytesSQL = "SELECT " +
		"(SELECT COALESCE(SUM(length(title) + length(intro) + length(source_path) + length(source_name) " +
		"+ length(encoding) + length(status)), 0) FROM books) + " +
		"(SELECT COALESCE(SUM(length(title) + length(content) + length(title_seg) + length(content_seg)), 0) FROM chapters)"

	indexBytesSQL = "SELECT " +
		"(SELECT COALESCE(SUM(length(block)), 0) FROM chapters_fts_data) + " +
		"(SELECT COALESCE(SUM(length(segid)), 0) FROM chapters_fts_idx) + " +
		"(SELECT COALESCE(SUM(length(sz)), 0) FROM chapters_fts_docsize) + " +
		"(SELECT COALESCE(SUM(length(k) + length(v)), 0) FROM chapters_fts_config)"
)

// estimateComposition 估算库体积构成，返回 textBytes / indexBytes。
//
// 任一查询失败时返回 (0, 0, err)，由调用方决定是否降级（Stats 里选择不因为
// 体积构成失败而整体失败：DBBytes 仍可给出）。
func (s *Store) estimateComposition(ctx context.Context) (textBytes, indexBytes int64, err error) {
	if err := s.db.QueryRowContext(ctx, textBytesSQL).Scan(&textBytes); err != nil {
		return 0, 0, fmt.Errorf("store: estimate text bytes: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, indexBytesSQL).Scan(&indexBytes); err != nil {
		return 0, 0, fmt.Errorf("store: estimate index bytes: %w", err)
	}
	return textBytes, indexBytes, nil
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
		// 估算超出实际占用：按比例缩放，保持比例可读且三者相加等于总大小。
		st.TextBytes = textBytes * st.DBBytes / total
		st.IndexBytes = indexBytes * st.DBBytes / total
		st.OtherBytes = st.DBBytes - st.TextBytes - st.IndexBytes
		return
	}
	st.TextBytes = textBytes
	st.IndexBytes = indexBytes
	st.OtherBytes = st.DBBytes - textBytes - indexBytes
}
