package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"norvels/internal/ai"
	"norvels/internal/store"
)

// 本文件补齐 worker 与文案层的剩余分支：AI 错误分类的每一档、写回失败只记日志、
// JSON 空切片的兜底。

// TestAIReasonBranches ai 包的每个分类错误都要有对应的中文原因。
func TestAIReasonBranches(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ai.ErrNoKey, "没有配置 API key"},
		{ai.ErrNoSegments, "没有可裁决的差异段落"},
		{fmt.Errorf("%w：超时", ai.ErrTimeout), "请求超时"},
		{fmt.Errorf("%w：A 与 B", ai.ErrInconsistent), "两次裁决结果不一致"},
		{errors.New("未知"), "未知"},
	}
	for _, tc := range cases {
		if got := aiReason(tc.err); got != tc.want {
			t.Errorf("aiReason(%v) = %q，期望 %q", tc.err, got, tc.want)
		}
	}
}

// TestWorkerSkipsAlreadyDecided 已被人工决策的记录不会被 worker 覆盖（写回失败只记日志）。
func TestWorkerSkipsAlreadyDecided(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	// 用户先点了保留：worker 的写回是条件更新，应当失败且不改状态。
	if _, code := decide(t, env, rev.ID, store.ReviewActionKeep); code != 200 {
		t.Fatalf("keep 状态码 = %d", code)
	}
	logs := 0
	w := &reviewWorker{
		st: env.st, judge: &fakeJudge{winners: []string{ai.WinnerB}},
		logf:    func(string, ...any) { logs++ },
		enabled: func(context.Context) bool { return true },
	}
	stored, err := env.st.GetImportReview(context.Background(), rev.ID)
	if err != nil {
		t.Fatalf("GetImportReview: %v", err)
	}
	w.processOne(context.Background(), stored)
	if logs == 0 {
		t.Errorf("写回失败没有记日志")
	}
	if items := listReviews(t, env, "all").Items; len(items) != 1 || items[0].Status != store.ReviewStatusConfirmedKeep {
		t.Errorf("状态被 worker 覆盖了：%+v", items)
	}
}

// TestNonNilHelpers JSON 兜底：nil 切片序列化成 []。
func TestNonNilHelpers(t *testing.T) {
	if got := nonNilStrings(nil); got == nil || len(got) != 0 {
		t.Errorf("nonNilStrings(nil) = %v，期望空切片", got)
	}
	if got := nonNilDims(nil); got == nil || len(got) != 0 {
		t.Errorf("nonNilDims(nil) = %v，期望空切片", got)
	}
}
