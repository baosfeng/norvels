package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/ai"
	"norvels/internal/store"
)

// 本文件补齐审核链路的异常分支与纯函数口径：请求体非法、AI 各类错误的降级文案、
// 结论映射、书被删后的列表形状、状态机错误消息。

// TestReviewDecideBodyValidation 决策请求体：空体 / 非 JSON / 缺 action 都是 400。
func TestReviewDecideBodyValidation(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	path := fmt.Sprintf("/api/import/reviews/%d/decide", rev.ID)
	cases := []struct {
		name string
		body string
	}{
		{"空请求体", ""},
		{"非法 JSON", "{"},
		{"缺 action", "{}"},
	}
	for _, tc := range cases {
		rr := env.do(http.MethodPost, path, strings.NewReader(tc.body),
			map[string]string{"Content-Type": "application/json"})
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
}

// TestReviewListAfterBookDeleted 书被删后列表仍返回记录（书摘要为空），不报错。
func TestReviewListAfterBookDeleted(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	rr := env.do(http.MethodDelete, fmt.Sprintf("/api/books/%d", rev.OldBook.ID), nil, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("删除旧书状态码 = %d，期望 204", rr.Code)
	}
	items := listReviews(t, env, "all").Items
	if len(items) != 1 {
		t.Fatalf("审核列表 = %d 条，期望 1 条（记录保留）", len(items))
	}
	if items[0].OldBook.ID != rev.OldBook.ID || items[0].OldBook.Title != "" {
		t.Errorf("旧书摘要 = %+v，期望只保留 id", items[0].OldBook)
	}
	if items[0].NewBook.ID != rev.NewBook.ID {
		t.Errorf("新书 id = %d，期望 %d", items[0].NewBook.ID, rev.NewBook.ID)
	}
}

// TestReviewWorkerAIErrorReasons AI 各类错误都要落 needs_manual，并给一句人话原因。
func TestReviewWorkerAIErrorReasons(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w：状态码 500", ai.ErrServer), "服务端错误"},
		{fmt.Errorf("%w：状态码 401", ai.ErrClient), "请求被拒绝"},
		{fmt.Errorf("%w：dial tcp", ai.ErrNetwork), "网络错误"},
		{fmt.Errorf("%w：不是 JSON", ai.ErrBadResponse), "不是合法的裁决 JSON"},
		{errors.New("奇怪的问题"), "奇怪的问题"},
	}
	for _, tc := range cases {
		env := newEnv(t)
		seedReviewPair(t, env)
		runWorker(t, env, &fakeJudge{err: tc.err})
		item := singleManual(t, env)
		if len(item.AI.Reasons) == 0 || !strings.Contains(item.AI.Reasons[0], tc.want) {
			t.Errorf("AI 错误 %v 的降级原因 = %v，期望包含 %q", tc.err, item.AI.Reasons, tc.want)
		}
	}
}

// TestReviewWorkerWinnerMapping AI 的三种结论都要映射成落库语义值（old / new / equal）。
func TestReviewWorkerWinnerMapping(t *testing.T) {
	cases := []struct{ winner, want string }{
		{ai.WinnerA, store.ReviewWinnerOld},
		{ai.WinnerB, store.ReviewWinnerNew},
		{ai.WinnerEqual, store.ReviewWinnerEqual},
	}
	for _, tc := range cases {
		env := newEnv(t)
		seedReviewPair(t, env)
		runWorker(t, env, &fakeJudge{winners: []string{tc.winner}})
		items := listReviews(t, env, "ready").Items
		if len(items) != 1 || items[0].AI.Winner != tc.want {
			t.Errorf("AI %q 落库 = %+v，期望 %q", tc.winner, items, tc.want)
		}
	}
}

// TestReviewStateMessage 状态机错误消息取「: 」之后的人话部分。
func TestReviewStateMessage(t *testing.T) {
	err := fmt.Errorf("%w: 审核记录 3 状态是 confirmed_keep，不能再 replace", store.ErrReviewState)
	if got := reviewStateMessage(err); !strings.Contains(got, "confirmed_keep") {
		t.Errorf("消息 = %q，期望带当前状态", got)
	}
	if got := reviewStateMessage(errors.New("没有冒号")); got == "" {
		t.Errorf("无冒号时应给通用文案，实际为空")
	}
	if lastColon("abc") != -1 {
		t.Errorf("lastColon(abc) = %d，期望 -1", lastColon("abc"))
	}
}

// TestReviewWorkerNilStore 空 store 时 worker 什么都不做（防御性分支）。
func TestReviewWorkerNilStore(t *testing.T) {
	w := &reviewWorker{logf: func(string, ...any) {}, enabled: func(context.Context) bool { return true }}
	if n := w.ProcessPending(context.Background()); n != 0 {
		t.Errorf("处理 %d 条，期望 0", n)
	}
}
