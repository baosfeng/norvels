package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"norvels/internal/model"
)

// 本文件是「书籍质量分」的落库口径（设计 §5）：books 的七列、读取与「未计算」判定。
//
// 质量分由解析链路算好放进 model.ParsedBook.Quality（只有 txt 链路有；EPUB 与合集拆出的
// 子书是零值）。落库时**零值不写成 0 分**——0 分是一份「完美」的质量分，会让版本择优
// 把一本没算过分的书当成「零乱码零噪声的干净版本」。因此「未计算」统一用
// quality_chapters = 0 且 quality_at = '' 表达，读取侧一律走 BookQuality.Computed()，
// 写入与读取不会各判一套。
//
// 七列都 NOT NULL DEFAULT 0/''：老库升级后存量行天然是「未计算」，由
// norvels backfill-quality 逐书补算（见 quality_backfill.go）。

// 质量分维度键（对比结果用它们指代维度，中文文案在 importer 侧拼）。
const (
	QualityDimGarbage   = "garbage"   // 乱码率
	QualityDimNoise     = "noise"     // 噪声率
	QualityDimStructure = "structure" // 结构分
)

// bookQualityColumn 是质量分的一列（列名 + 建列 DDL）。
type bookQualityColumn struct {
	name string
	ddl  string
}

// bookQualityColumns 是 0020 的七列，顺序与 qualityArgs 一致（列名在这里，SQL 在
// book_write.go 的写入语句里，改口径两处一起改——测试会按写入→读回断言两者一致）。
var bookQualityColumns = []bookQualityColumn{
	{"garbage_ratio", "garbage_ratio REAL NOT NULL DEFAULT 0"},
	{"noise_ratio", "noise_ratio REAL NOT NULL DEFAULT 0"},
	{"structure_score", "structure_score REAL NOT NULL DEFAULT 0"},
	{"duplicate_ratio", "duplicate_ratio REAL NOT NULL DEFAULT 0"},
	{"quality_chapters", "quality_chapters INTEGER NOT NULL DEFAULT 0"},
	{"quality_chars", "quality_chars INTEGER NOT NULL DEFAULT 0"},
	{"quality_at", "quality_at TEXT NOT NULL DEFAULT ''"},
}

// 写入语句里质量分那一段（与 bookQualityColumns 同序，与 qualityArgs 同序）。
const (
	bookQualityInsertColumns = "garbage_ratio, noise_ratio, structure_score, duplicate_ratio, " +
		"quality_chapters, quality_chars, quality_at"
	bookQualityPlaceholders      = "?, ?, ?, ?, ?, ?, ?"
	bookQualityUpdateAssignments = "garbage_ratio = ?, noise_ratio = ?, structure_score = ?, " +
		"duplicate_ratio = ?, quality_chapters = ?, quality_chars = ?, quality_at = ?"
)

// BookQuality 是一本书的质量分（books 的七列）。
//
// 比率类字段范围 0~1：除 StructureScore 外都是「越低越好」；计数类字段供「完整性对照」
// 与界面展示（如「乱码 120 处 → 0 处」）。
type BookQuality struct {
	GarbageRatio   float64   `json:"garbageRatio"`
	NoiseRatio     float64   `json:"noiseRatio"`
	StructureScore float64   `json:"structureScore"`
	DuplicateRatio float64   `json:"duplicateRatio"`
	Chapters       int       `json:"chapters"`
	Chars          int64     `json:"chars"`
	ComputedAt     time.Time `json:"-"` // 零值 = 未计算
}

// Computed 报告这份质量分是不是算过的。
//
// 判据是「章数 > 0 且时间戳非零」两条同时成立：只写其一是半截状态（迁移与写入都是
// 七列一起写），读到时按未计算处理，宁可让版本择优走保守路径，也不拿半份分数做判定。
func (q BookQuality) Computed() bool { return q.Chapters > 0 && !q.ComputedAt.IsZero() }

// qualityFromMetrics 把解析期质量分翻译成落库形状；未计算返回零值（= 未计算）。
//
// ChapterCount 是解析链路算质量分时一定填的（等于参与统计的章数），它是「算过没有」的
// 唯一信号：比率类的 0 既可能是「干净」也可能是「没算」，章数不会。
func qualityFromMetrics(m model.QualityMetrics, at time.Time) BookQuality {
	if m.ChapterCount <= 0 {
		return BookQuality{}
	}
	return BookQuality{
		GarbageRatio:   m.GarbageRatio,
		NoiseRatio:     m.NoiseRatio,
		StructureScore: m.StructureScore,
		DuplicateRatio: m.DuplicateRatio,
		Chapters:       m.ChapterCount,
		Chars:          int64(m.TotalChars),
		ComputedAt:     at,
	}
}

// qualityArgs 返回七列的写入参数（顺序与 bookQualityColumns 一致）。
//
// 未计算的书整行写零值 + 空串：与 DEFAULT 同值，老库与新库读到的「未计算」是同一个形状。
func (q BookQuality) qualityArgs() []any {
	if !q.Computed() {
		return []any{0.0, 0.0, 0.0, 0.0, 0, int64(0), ""}
	}
	return []any{q.GarbageRatio, q.NoiseRatio, q.StructureScore, q.DuplicateRatio,
		q.Chapters, q.Chars, formatTime(q.ComputedAt)}
}

// bookQualitySelectSQL 是读取七列的语句（按主键，一次索引查找）。
const bookQualitySelectSQL = "SELECT garbage_ratio, noise_ratio, structure_score, duplicate_ratio, " +
	"quality_chapters, quality_chars, quality_at FROM books WHERE id = ?"

// bookQualityByID 读一本书的质量分；书不存在返回零值（= 未计算），不报错。
//
// 「书不存在」在判重路径上是合法情形（指纹成了孤儿），此时版本择优没有可比对象，
// 由调用方走保守路径——不该因为一本已删的书让整批导入失败。
func (s *Store) bookQualityByID(ctx context.Context, id int64) (BookQuality, error) {
	var q BookQuality
	var at string
	err := s.db.QueryRowContext(ctx, bookQualitySelectSQL, id).Scan(&q.GarbageRatio, &q.NoiseRatio,
		&q.StructureScore, &q.DuplicateRatio, &q.Chapters, &q.Chars, &at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return BookQuality{}, nil
	case err != nil:
		return BookQuality{}, fmt.Errorf("store: read book quality %d: %w", id, err)
	}
	if t, perr := parseTime(at); perr == nil {
		q.ComputedAt = t
	}
	return q, nil
}

// BookQuality 读一本书的质量分（导出给 API 与导入链路诊断用）。
//
// 未计算的书返回零值（BookQuality.Computed() == false），不报错：调用方按
// 「有没有质量分」分支，而不是按错误分支。
func (s *Store) BookQuality(ctx context.Context, bookID int64) (BookQuality, error) {
	return s.bookQualityByID(ctx, bookID)
}

// QualityCoverage 是「已算 / 未算」两档书数（健康报告用）。
type QualityCoverage struct {
	Books    int // 主库书数
	Computed int // 有质量分的书数
	Missing  int // 缺口 = Books − Computed
}

// qualityCoverage 统计质量分覆盖率。
//
// 「算过」的判据与 BookQuality.Computed() 同口径（章数 > 0 且时间戳非空），
// 写成 SQL 条件是为了走一次全表扫描而不是把两万行读进 Go。
func (s *Store) qualityCoverage(ctx context.Context) (QualityCoverage, error) {
	var c QualityCoverage
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN quality_chapters > 0 AND quality_at <> '' "+
			"THEN 1 ELSE 0 END), 0) FROM books").Scan(&c.Books, &c.Computed)
	if err != nil {
		return QualityCoverage{}, fmt.Errorf("store: quality coverage: %w", err)
	}
	c.Missing = c.Books - c.Computed
	return c, nil
}
