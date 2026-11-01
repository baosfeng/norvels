package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"norvels/internal/model"
)

// 本文件是「章级去重判定」的读路径（设计 §4）：新书逐章算指纹 → 分批查正文库 → 按字节加权算命中率。
//
// 判定只看**正文内容**：文件名、路径、书名、编码都不参与。分档结论由 DedupeReport.Level 给出，
// 阈值由调用方注入（settings 的 dedupe.threshold，缺省 0.95）；「质量分对比 / 升级候选」
// 是下一阶段（设计 §5），报告里先留好字段位置，判定不读它们。

// relatedHitRate 是「疑似相关」的下界（设计 §4 表格）：低于它的命中率按新书处理。
const relatedHitRate = 0.40

// dedupeBatchSize 是每批查询的哈希个数：SQLite 的变量上限是 999，
// 一批 500 个哈希对两千章的书只发 4 条 SQL，又不会撞上限。
const dedupeBatchSize = 500

// 去重结论的三档（设计 §4 表格）。
const (
	DedupeLevelNovel     = "novel"     // < 40%：新书，正常导入
	DedupeLevelRelated   = "related"   // 40% ~ 阈值：疑似相关，正常导入 + 备注
	DedupeLevelDuplicate = "duplicate" // ≥ 阈值：视为同一本书，按重复跳过
)

// DedupeReport 是一次章级去重判定的结果。
type DedupeReport struct {
	ChapterCount int     // 参与判定的章节数（空白章不计入）
	TotalBytes   int64   // 参与判定的章节正文原始字节合计
	HitBytes     int64   // 其中在库内已存在相同指纹的字节
	HitRate      float64 // HitBytes / TotalBytes；TotalBytes 为 0 时是 0
	// DuplicateBookID 是命中字节最多的书 id（0 = 库内没有相同章节）；
	// DuplicateBookTitle 是它的书名（书已被删、指纹成孤儿时为空串）。
	DuplicateBookID    int64
	DuplicateBookTitle string
	DuplicateBytes     int64 // 命中最多的那本书贡献的命中字节
	DuplicateChapters  int   // 命中最多的那本书命中的章节数

	// 下一阶段（设计 §5）质量分对比的占位：nil 表示尚未计算。
	// 现在只有字段形状，判定不读它们——预留下来是为了加质量分时不必改 CheckContentDuplicate 的签名。
	DuplicateQuality *QualityScore
	NewQuality       *QualityScore
}

// QualityScore 是「版本择优」要对比的质量分维度（设计 §5），当前只用于占位。
type QualityScore struct {
	GarbledRatio   float64 // 乱码占比（U+FFFD / 私用区），越低越好
	NoiseRatio     float64 // 广告/水印/网址行占比，越低越好
	StructureScore float64 // countSanity + 空章率 + 离群章率的合成
	ChapterCount   int     // 完整性对照：章数
	WordCount      int64   // 完整性对照：字数
}

// Level 按阈值给出本报告的分档结论；阈值越界（≤0 或 >1）时回落 DefaultDedupeThreshold。
//
// 分档只写在这里：importer 负责把结论翻译成「跳过 / 备注」，不重复定义阈值与百分比。
func (r DedupeReport) Level(threshold float64) string {
	if threshold <= 0 || threshold > 1 {
		threshold = DefaultDedupeThreshold
	}
	switch {
	case r.TotalBytes <= 0:
		return DedupeLevelNovel
	case r.HitRate >= threshold:
		return DedupeLevelDuplicate
	case r.HitRate >= relatedHitRate:
		return DedupeLevelRelated
	}
	return DedupeLevelNovel
}

// CheckContentDuplicate 判定一批章节（一般是待导入的一本书）在库内的重复程度。
//
// 报告只描述「命中了多少」，不决定导入行为：跳过还是带备注导入由调用方按 Level 决定
// （importer 的判定见 internal/importer/dedupe.go）。空章节不参与判定——它们不带内容，
// 参与的话所有书的空章会互相命中，把命中率顶到假的阈值上。
//
// 正文库还没有指纹表（迁移 0019 未应用）时退化为「不判重」并记一条 WARN：
// 判重是提示与加速，不该把整批导入拦下来。
func (s *Store) CheckContentDuplicate(ctx context.Context, chapters []model.ParsedChapter) (DedupeReport, error) {
	fps := chapterFingerprints(chapters)
	rep := DedupeReport{ChapterCount: len(fps)}
	for _, fp := range fps {
		rep.TotalBytes += fp.bytes
	}
	if rep.TotalBytes <= 0 {
		return rep, nil
	}
	ready, err := s.fingerprintTableExists(ctx)
	if err != nil {
		return DedupeReport{}, err
	}
	if !ready {
		log.Printf("WARN: 正文库没有 chapter_fingerprints（迁移 0019 未应用），本次不判重")
		return rep, nil
	}
	hits, err := s.lookupFingerprints(ctx, uniqueHashes(fps))
	if err != nil {
		return DedupeReport{}, err
	}
	st := aggregateHits(fps, hits)
	rep.HitBytes = st.bytes
	rep.HitRate = hitRate(st.bytes, rep.TotalBytes)
	rep.DuplicateBookID, rep.DuplicateBytes, rep.DuplicateChapters = st.bookID, st.bookBytes, st.chapters
	if rep.DuplicateBookID == 0 {
		return rep, nil
	}
	if rep.DuplicateBookTitle, err = s.bookTitleByID(ctx, rep.DuplicateBookID); err != nil {
		return DedupeReport{}, err
	}
	return rep, nil
}

// chapterFP 是一章的指纹与字节权重。
type chapterFP struct {
	hash  int64
	bytes int64
}

// chapterFingerprints 逐章算指纹，跳过空白章。
func chapterFingerprints(chapters []model.ParsedChapter) []chapterFP {
	out := make([]chapterFP, 0, len(chapters))
	for i := range chapters {
		hash, bytes, ok := chapterFingerprint(chapters[i].Content)
		if !ok {
			continue
		}
		out = append(out, chapterFP{hash: hash, bytes: bytes})
	}
	return out
}

// uniqueHashes 返回去重后的哈希列表：同一本书里内容完全相同的章节只需查一次。
func uniqueHashes(fps []chapterFP) []int64 {
	seen := make(map[int64]struct{}, len(fps))
	out := make([]int64, 0, len(fps))
	for _, fp := range fps {
		if _, dup := seen[fp.hash]; dup {
			continue
		}
		seen[fp.hash] = struct{}{}
		out = append(out, fp.hash)
	}
	return out
}

// lookupFingerprints 分批查库内命中，返回 hash → 命中它的书列表。
func (s *Store) lookupFingerprints(ctx context.Context, hashes []int64) (map[int64][]fingerprintHit, error) {
	out := make(map[int64][]fingerprintHit, len(hashes))
	for start := 0; start < len(hashes); start += dedupeBatchSize {
		end := min(start+dedupeBatchSize, len(hashes))
		if err := s.scanFingerprintBatch(ctx, hashes[start:end], out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// scanFingerprintBatch 查一批哈希（调用方保证 len(batch) ∈ [1, dedupeBatchSize]）。
func (s *Store) scanFingerprintBatch(ctx context.Context, batch []int64, out map[int64][]fingerprintHit) error {
	args := make([]any, 0, len(batch))
	for _, h := range batch {
		args = append(args, h)
	}
	query := "SELECT hash, book_id, bytes FROM chapter_fingerprints WHERE hash IN (" + placeholders(len(batch)) + ")"
	rows, err := s.cdb.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: query fingerprints: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	for rows.Next() {
		var (
			hash int64
			hit  fingerprintHit
		)
		if err := rows.Scan(&hash, &hit.book, &hit.bytes); err != nil {
			return fmt.Errorf("store: scan fingerprint: %w", err)
		}
		out[hash] = append(out[hash], hit)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: query fingerprints: %w", err)
	}
	return nil
}

// fingerprintHit 是一条「库内已有」命中：哪本书、书里那一章占多少字节。
//
// 字节数按**新书这一章**的重量累计（见 aggregateHits），库内那行 bytes 只作为
// 「库内版本有多大」的参照，下一阶段质量分对比会用到它。
type fingerprintHit struct {
	book  int64
	bytes int64
}

// hitStats 是命中聚合结果：总命中字节与命中最多的那本书。
type hitStats struct {
	bytes     int64
	bookID    int64
	bookBytes int64
	chapters  int
}

// aggregateHits 把逐章命中折算成「命中字节 + 命中最多的书」。
//
// 命中字节按新书的章节重量累加（同一章只算一次，不管库内有几本书含它）：
// 命中率的语义是「这本书有多少内容库内已经有」，与该内容在库内出现了几次无关。
// 逐书累计同样按新书章节重量，这样「命中最多的书」与命中率是同一个口径。
func aggregateHits(fps []chapterFP, hits map[int64][]fingerprintHit) hitStats {
	var st hitStats
	bytesByBook := make(map[int64]int64, 4)
	chaptersByBook := make(map[int64]int, 4)
	for _, fp := range fps {
		books := hits[fp.hash]
		if len(books) == 0 {
			continue
		}
		st.bytes += fp.bytes
		for _, hit := range books {
			bytesByBook[hit.book] += fp.bytes
			chaptersByBook[hit.book]++
		}
	}
	return pickTopBook(st, bytesByBook, chaptersByBook)
}

// pickTopBook 选命中字节最多的书；并列时取 id 小的，保证同一输入每次判定结果一致。
func pickTopBook(st hitStats, bytesByBook map[int64]int64, chaptersByBook map[int64]int) hitStats {
	for bookID, bytes := range bytesByBook {
		better := bytes > st.bookBytes || (bytes == st.bookBytes && (st.bookID == 0 || bookID < st.bookID))
		if !better {
			continue
		}
		st.bookID, st.bookBytes, st.chapters = bookID, bytes, chaptersByBook[bookID]
	}
	return st
}

// hitRate 返回命中率，夹在 [0,1]（浮点累计误差不许超过 100%）。
func hitRate(hit, total int64) float64 {
	if total <= 0 {
		return 0
	}
	if rate := float64(hit) / float64(total); rate < 1 {
		return rate
	}
	return 1
}

// bookTitleByID 取书名供提示文案用；书已被删（指纹成孤儿）时不报错，返回空串。
func (s *Store) bookTitleByID(ctx context.Context, id int64) (string, error) {
	var title string
	err := s.db.QueryRowContext(ctx, "SELECT title FROM books WHERE id = ?", id).Scan(&title)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("store: dedupe book title %d: %w", id, err)
	}
	return title, nil
}
