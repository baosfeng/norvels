package ai

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// JudgeTwice 对同一请求判定两次并做一致性校验：两次 winner 相同才采纳。
//
// 两次调用各自独立随机化 A/B 呈现顺序（由 Judge 实现负责），因此「一致」等于
// 「结论不依赖呈现位置」；不一致说明模型结论不可重复，按设计降级为规则结论 + 人工确认。
// 采纳的置信度取两次中较低的一个（保守），理由按先首轮后次轮去重合并。
func JudgeTwice(ctx context.Context, j Judge, req Request) (Verdict, error) {
	first, err := j.Judge(ctx, req)
	if err != nil {
		return Verdict{}, err
	}
	second, err := j.Judge(ctx, req)
	if err != nil {
		return Verdict{}, err
	}
	if first.Winner != second.Winner {
		return Verdict{}, fmt.Errorf("%w：%s 与 %s", ErrInconsistent, first.Winner, second.Winner)
	}
	return mergeVerdicts(first, second), nil
}

// JudgeTwice 用本裁决器做两次一致性判定：只是 ai.JudgeTwice(ctx, c, req) 的薄封装
// （共用同一段逻辑，不重复实现）。直接用 ChatJudge 的调用方走这个方法少传一个参数；
// 面向 Judge 接口编程（含测试替身）时用包级 ai.JudgeTwice。
func (c *ChatJudge) JudgeTwice(ctx context.Context, req Request) (Verdict, error) {
	return JudgeTwice(ctx, c, req)
}

// mergeVerdicts 合并两次 winner 相同的判定。
func mergeVerdicts(a, b Verdict) Verdict {
	return Verdict{
		Winner:     a.Winner,
		Confidence: math.Min(a.Confidence, b.Confidence),
		Reasons:    dedupeReasons(a.Reasons, b.Reasons),
	}
}

// dedupeReasons 合并多组理由并保序去重。
func dedupeReasons(groups ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, g := range groups {
		for _, r := range g {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			out = append(out, r)
		}
	}
	return out
}
