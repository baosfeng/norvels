package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 本文件是审核记录的扫描与编码辅助：行扫描、理由 JSON、参数归一化。
// 单独成文件是为了让 reviews.go 保持在「类型 + 读写入口」的规模内（门禁 5 的 400 行上限）。

// scanReview 扫一行审核记录。
//
// 时间与 JSON 列容错：解析失败留零值 / 空切片，不让一条脏行废掉整页
// （与 applySummaryTimes 同一取舍）。
func scanReview(sc rowScanner) (ImportReview, error) {
	var (
		rev     ImportReview
		created string
		decided string
		aiRaw   string
		ruleRaw string
	)
	if err := sc.Scan(&rev.ID, &rev.JobID, &rev.NewBookID, &rev.OldBookID, &rev.HitRatio,
		&rev.Status, &rev.AIWinner, &rev.AIConfidence, &aiRaw, &ruleRaw,
		&created, &decided, &rev.DecidedBy); err != nil {
		return rev, err
	}
	rev.CreatedAt, _ = parseTime(created)
	if t, perr := parseTime(decided); perr == nil && !t.IsZero() {
		rev.DecidedAt = &t
	}
	rev.AIReasons = decodeReasons(aiRaw)
	rev.RuleReasons = decodeReasons(ruleRaw)
	return rev, nil
}

// scanReviewBook 扫一行审核列表里的书。
func scanReviewBook(sc rowScanner) (ReviewBook, error) {
	var (
		b  ReviewBook
		at string
	)
	if err := sc.Scan(&b.ID, &b.Title, &b.ChapterCount, &b.WordCount, &b.SupersededBy,
		&b.Quality.GarbageRatio, &b.Quality.NoiseRatio, &b.Quality.StructureScore,
		&b.Quality.DuplicateRatio, &b.Quality.Chapters, &b.Quality.Chars, &at); err != nil {
		return b, fmt.Errorf("store: scan review book: %w", err)
	}
	if t, err := parseTime(at); err == nil {
		b.Quality.ComputedAt = t
	}
	return b, nil
}

// normalizeAIResult 校验 worker 写回的结论形状。
func normalizeAIResult(res ReviewAIResult) (status, winner string, reasons []string, err error) {
	switch res.Status {
	case ReviewStatusReady:
		switch res.Winner {
		case ReviewWinnerNew, ReviewWinnerOld, ReviewWinnerEqual:
		default:
			return "", "", nil, fmt.Errorf("%w: AI 结论 %q 不是 new/old/equal", ErrInvalidArgument, res.Winner)
		}
		return ReviewStatusReady, res.Winner, res.Reasons, nil
	case ReviewStatusNeedsManual:
		return ReviewStatusNeedsManual, "", res.Reasons, nil
	default:
		return "", "", nil, fmt.Errorf("%w: AI 写回状态 %q 只支持 ready / needs_manual",
			ErrInvalidArgument, res.Status)
	}
}

// encodeReasons 把理由切片编码成 JSON（nil 编码成 []，前端不必判 null）。
func encodeReasons(reasons []string) (string, error) {
	if reasons == nil {
		reasons = []string{}
	}
	raw, err := json.Marshal(reasons)
	if err != nil {
		return "", fmt.Errorf("store: encode review reasons: %w", err)
	}
	return string(raw), nil
}

// decodeReasons 解码理由 JSON；坏数据按空切片处理。
//
// 审核列表是展示路径：一条脏行不该让整页 500（与 applySummaryTimes 同一取舍）。
func decodeReasons(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// uniquePositiveIDs 去重并丢掉非正数 id（保持首次出现的顺序）。
func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// clampRatio 把 0~1 的比率夹进合法区间（浮点累计误差不该写进库）。
func clampRatio(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
