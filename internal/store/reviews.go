package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 本文件是 AI 审核记录（设计 §6）的类型、落库与读取；状态机与替换语义在
// reviews_decide.go，扫描与编码辅助在 reviews_scan.go。
//
// # 为什么新版本要「先入库再隐藏」
//
// 判出「升级候选」时，新版本照常写入（章节 / 正文块 / 质量分都在库里），但立刻把
// books.superseded_by 指向旧书 —— 书架与列表因此不会多出一本重复的书，而两版正文
// 都在库里：替换与回滚都只改两个整数字段，不需要重新导入任何文件。
//
// # 状态机（非法迁移一律 ErrReviewState，api 映射 409）
//
//	pending ──AI 完成──→ ready
//	   │                    │
//	   └──AI 不可用/失败──→ needs_manual
//	                        │
//	pending / ready / needs_manual ──decide──→ confirmed_replace | confirmed_keep
//	                                                  │
//	                          confirmed_replace ──rollback──→ rolled_back
//
// 为什么允许 pending 直接 decide：AI 只是建议（设计 §6），用户不必等它；worker 的
// 写回是条件更新（WHERE status = 'pending'），被用户抢先时不会覆盖用户的选择。
// 为什么 keep 只改状态：新版本入库时就已是 superseded（不可见），保留旧版本不需要
// 再动任何一本书。

// 审核状态（import_reviews.status）。
const (
	ReviewStatusPending          = "pending"
	ReviewStatusReady            = "ready"
	ReviewStatusNeedsManual      = "needs_manual"
	ReviewStatusConfirmedKeep    = "confirmed_keep"
	ReviewStatusConfirmedReplace = "confirmed_replace"
	ReviewStatusRolledBack       = "rolled_back"
	// ReviewStatusAll 是「不按状态筛」的哨兵值（GET /api/import/reviews?status=all）。
	ReviewStatusAll = "all"
)

// AI 结论的三档（落 ai_winner 列；空串 = 没有 AI 结论）。
//
// 用 new / old / equal 而不是 A / B：A/B 是 ai 包内部的呈现标签（每次调用随机互换），
// 落库与对外的字段必须是与呈现顺序无关的语义值。
const (
	ReviewWinnerNew   = "new"
	ReviewWinnerOld   = "old"
	ReviewWinnerEqual = "equal"
)

// 决策动作（POST /api/import/reviews/{id}/decide 的 action）。
const (
	ReviewActionReplace = "replace"
	ReviewActionKeep    = "keep"
)

// reviewActionRollback 是内部动作名（对外只有 replace / keep 两个 action）。
const reviewActionRollback = "rollback"

// activeBookCond 是「这本书是当前版本」的 SQL 条件（别名 b = books）。
//
// 0021 起「书」的默认口径只看当前版本：书架 / 搜索 / 详情 / 阅读都带这个条件，
// 被替换掉的旧版本只在 AI 审核页可见（那里按 id 直查，不过滤）。
const activeBookCond = "b.superseded_by = 0"

// 审核列表的条数口径。
const (
	defaultReviewLimit = 50
	maxReviewLimit     = 200
)

// reviewColumns 是 ImportReview 的取数列（顺序与 scanReview 的扫描目标一致）。
const reviewColumns = "id, job_id, new_book_id, old_book_id, hit_ratio, status, " +
	"ai_winner, ai_confidence, ai_reasons, rule_reasons, created_at, decided_at, decided_by"

// ImportReview 是一条审核记录。
//
// HitRatio 是判定「同一本书」时的章级命中率；AIWinner / AIConfidence / AIReasons 是
// 裁决结果（needs_manual 时 AIReasons 写「为什么没有 AI 结论」，可能为空）；
// RuleReasons 是导入时算出的规则对比文案（如「噪声率 3.2%→0.4%」），不随状态变化。
type ImportReview struct {
	ID           int64      `json:"id"`
	JobID        int64      `json:"jobId"`
	NewBookID    int64      `json:"newBookId"`
	OldBookID    int64      `json:"oldBookId"`
	HitRatio     float64    `json:"hitRatio"`
	Status       string     `json:"status"`
	AIWinner     string     `json:"aiWinner,omitempty"`
	AIConfidence float64    `json:"aiConfidence"`
	AIReasons    []string   `json:"aiReasons"`
	RuleReasons  []string   `json:"ruleReasons"`
	CreatedAt    time.Time  `json:"createdAt"`
	DecidedAt    *time.Time `json:"decidedAt,omitempty"`
	DecidedBy    string     `json:"decidedBy"`
}

// NewReview 是导入判出「升级候选」时要落的一条审核记录。
//
// 调用方在 SaveBook 拿到新书 id 之后立刻调用：新书会同时被标记为 superseded
// （不可见），两件事在同一个事务里，不会出现「书可见了但审核记录没落」。
type NewReview struct {
	JobID       int64    // 触发这次审核的导入任务（0 = 无任务）
	NewBookID   int64    // 新版本（已入库，将被隐藏）
	OldBookID   int64    // 库内当前版本
	HitRatio    float64  // 章级命中率
	RuleReasons []string // 规则对比文案（显著更优的维度）
}

// ReviewAIResult 是后台 worker 写回的一次裁决结果。
type ReviewAIResult struct {
	Status     string   // ready（有结论）/ needs_manual（降级）
	Winner     string   // new / old / equal；needs_manual 时留空
	Confidence float64  // 0..1
	Reasons    []string // AI 依据；needs_manual 时是「为什么没有结论」
}

// ReviewBook 是审核列表里展示的一本书（标题 + 质量分 + 当前是否被取代）。
//
// SupersededBy 是 books.superseded_by 的原值：0 = 当前版本（书架可见），
// 非 0 = 被这个 id 取代（隐藏）。列表页靠它标出「哪一版正在用」。
type ReviewBook struct {
	ID           int64       `json:"id"`
	Title        string      `json:"title"`
	ChapterCount int         `json:"chapterCount"`
	WordCount    int         `json:"wordCount"`
	SupersededBy int64       `json:"supersededBy"`
	Quality      BookQuality `json:"quality"`
}

// CreateImportReview 落一条 pending 审核记录，并把新书标记为「被旧书取代」（隐藏）。
//
// 两件事必须同事务：只落记录不隐藏，书架会立刻多出一本重复的书；只隐藏不落记录，
// 这本书就再也不会出现在任何界面里。任一失败整体回滚，调用方按「审核不可用」降级
// （新版本按普通书入库、可见），导入本身不受影响。
func (s *Store) CreateImportReview(ctx context.Context, in NewReview) (int64, error) {
	if err := validateNewReview(in); err != nil {
		return 0, err
	}
	reasons, err := encodeReasons(in.RuleReasons)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin create review: %w", err)
	}
	return commitReviewTx(ctx, tx, in, reasons)
}

// validateNewReview 校验新旧书 id：两个都必须有、且不能是同一本。
func validateNewReview(in NewReview) error {
	if in.NewBookID > 0 && in.OldBookID > 0 && in.NewBookID != in.OldBookID {
		return nil
	}
	return fmt.Errorf("%w: 审核记录的新旧书 id 非法（new=%d old=%d）",
		ErrInvalidArgument, in.NewBookID, in.OldBookID)
}

// commitReviewTx 在事务里写书侧与记录，成功才提交；失败整体回滚。
func commitReviewTx(ctx context.Context, tx *sql.Tx, in NewReview, reasons string) (int64, error) {
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	id, err := insertReviewTx(ctx, tx, in, reasons)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit import review: %w", err)
	}
	committed = true
	return id, nil
}

// insertReviewTx 隐藏新书并插入 pending 记录（两件事必须同事务）。
func insertReviewTx(ctx context.Context, tx *sql.Tx, in NewReview, reasons string) (int64, error) {
	// 旧书必须还在：它被删了的话「被谁取代」就指向一个不存在的 id，新书会永久不可见。
	if err := requireBookTx(ctx, tx, in.OldBookID); err != nil {
		return 0, err
	}
	if err := setSupersededBy(ctx, tx, in.NewBookID, in.OldBookID); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO import_reviews(job_id, new_book_id, old_book_id, hit_ratio, status, "+
			"rule_reasons, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		in.JobID, in.NewBookID, in.OldBookID, clampRatio(in.HitRatio),
		ReviewStatusPending, reasons, formatTime(nowUTC()))
	if err != nil {
		return 0, fmt.Errorf("store: insert import review: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: import review id: %w", err)
	}
	return id, nil
}

// ListImportReviews 按状态列审核记录（status 为空或 "all" 表示不筛），最新的在前。
func (s *Store) ListImportReviews(ctx context.Context, status string, limit int) ([]ImportReview, error) {
	where, args := reviewStatusFilter(status)
	return s.queryReviews(ctx, where, args, "created_at DESC, id DESC", limit)
}

// PendingImportReviews 取待 AI 裁决的记录，最旧的在前（先到先审，不会饿死）。
func (s *Store) PendingImportReviews(ctx context.Context, limit int) ([]ImportReview, error) {
	return s.queryReviews(ctx, []string{"status = ?"}, []any{ReviewStatusPending}, "created_at ASC, id ASC", limit)
}

// GetImportReview 读一条审核记录；不存在返回 ErrNotFound。
func (s *Store) GetImportReview(ctx context.Context, id int64) (ImportReview, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+reviewColumns+" FROM import_reviews WHERE id = ?", id)
	rev, err := scanReview(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ImportReview{}, fmt.Errorf("%w: import review %d", ErrNotFound, id)
	case err != nil:
		return ImportReview{}, fmt.Errorf("store: get import review %d: %w", id, err)
	}
	return rev, nil
}

// BooksForReview 批量取审核列表要展示的书（标题 / 章数字数 / 质量分 / superseded_by）。
//
// 不走书籍列表的「只取当前版本」过滤：审核要对照的正是「被隐藏的旧版本 / 新版本」，
// 用 ListBooks 一条都查不到。
func (s *Store) BooksForReview(ctx context.Context, ids []int64) (map[int64]ReviewBook, error) {
	out := make(map[int64]ReviewBook, len(ids))
	uniq := uniquePositiveIDs(ids)
	if len(uniq) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(uniq))
	for _, id := range uniq {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, title, chapter_count, word_count, superseded_by, garbage_ratio, noise_ratio, "+
			"structure_score, duplicate_ratio, quality_chapters, quality_chars, quality_at "+
			"FROM books WHERE id IN ("+placeholders(len(uniq))+")", args...)
	if err != nil {
		return nil, fmt.Errorf("store: list review books: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	for rows.Next() {
		b, err := scanReviewBook(rows)
		if err != nil {
			return nil, err
		}
		out[b.ID] = b
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list review books: %w", err)
	}
	return out, nil
}

// queryReviews 是列表与待办的共用查询。
func (s *Store) queryReviews(ctx context.Context, where []string, args []any, order string, limit int) ([]ImportReview, error) {
	q := "SELECT " + reviewColumns + " FROM import_reviews"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY " + order + " LIMIT ?"
	qargs := append(append([]any{}, args...), normReviewLimit(limit))
	rows, err := s.db.QueryContext(ctx, q, qargs...)
	if err != nil {
		return nil, fmt.Errorf("store: list import reviews: %w", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	items := make([]ImportReview, 0, 8)
	for rows.Next() {
		rev, err := scanReview(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan import review: %w", err)
		}
		items = append(items, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list import reviews: %w", err)
	}
	return items, nil
}

// reviewStatusFilter 把查询参数翻成 WHERE 片段（空 / all = 不筛，其余按字面匹配）。
//
// 未知值不报 400 是有意的：列表是只读展示路径，前端传错一个枚举值只会拿到空列表；
// 真正的状态机约束在 decide / rollback 上（那里非法迁移一律 409）。
func reviewStatusFilter(status string) ([]string, []any) {
	st := strings.TrimSpace(status)
	if st == "" || st == ReviewStatusAll {
		return nil, nil
	}
	return []string{"status = ?"}, []any{st}
}

// normReviewLimit 归一化条数（默认 50，上限 200）。
func normReviewLimit(limit int) int {
	if limit <= 0 {
		return defaultReviewLimit
	}
	if limit > maxReviewLimit {
		return maxReviewLimit
	}
	return limit
}
