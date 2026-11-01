package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"norvels/internal/ai"
	"norvels/internal/store"
)

// 本文件是 worker 侧与状态机的证据：AI 不可用时的三种降级（无 key / 开关关 / 两次不一致）、
// keep 的语义、非法迁移一律 409，以及「没有 AI key 时导入完全不受影响」的对照。

// TestReviewNeedsManualWithoutKey 没有 key：落 needs_manual，带人话原因，书架不变。
func TestReviewNeedsManualWithoutKey(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	if n := runWorker(t, env, nil); n != 1 {
		t.Fatalf("worker 处理 %d 条，期望 1 条", n)
	}
	item := singleManual(t, env)
	if item.AI.Winner != "" || len(item.AI.Reasons) == 0 {
		t.Errorf("降级结论 = %+v，期望没有 winner 但带原因", item.AI)
	}
	if !strings.Contains(item.AI.Reasons[0], "NORVELS_AI_API_KEY") {
		t.Errorf("降级原因 = %q，期望说明缺少 key", item.AI.Reasons[0])
	}
	assertOnlyBook(t, env, rev.OldBook.ID)
	// 降级后仍可人工决策（AI 只是建议）。
	if _, code := decide(t, env, rev.ID, store.ReviewActionKeep); code != http.StatusOK {
		t.Errorf("needs_manual 下决策状态码 = %d，期望 200", code)
	}
}

// singleManual 取唯一一条 needs_manual 记录。
func singleManual(t *testing.T, env *testEnv) reviewItem {
	t.Helper()
	items := listReviews(t, env, "needs_manual").Items
	if len(items) != 1 {
		t.Fatalf("needs_manual 列表 = %d 条，期望 1 条", len(items))
	}
	return items[0]
}

// TestReviewNeedsManualWhenAIDisabled 开关关闭：即使有裁决器也不外发。
func TestReviewNeedsManualWhenAIDisabled(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	judge := &fakeJudge{winners: []string{ai.WinnerB}}
	w := &reviewWorker{
		st: env.st, judge: judge,
		logf:    func(string, ...any) {},
		enabled: func(context.Context) bool { return false },
	}
	if n := w.ProcessPending(context.Background()); n != 1 {
		t.Fatalf("worker 处理 %d 条，期望 1 条", n)
	}
	if len(judge.reqs) != 0 {
		t.Errorf("开关关闭时仍发起了 %d 次裁决，期望 0", len(judge.reqs))
	}
	item := singleManual(t, env)
	if !strings.Contains(item.AI.Reasons[0], "ai.enabled") {
		t.Errorf("降级原因 = %q，期望说明开关关闭", item.AI.Reasons[0])
	}
	if item.ID != rev.ID {
		t.Errorf("降级记录 id = %d，期望 %d", item.ID, rev.ID)
	}
}

// TestReviewNeedsManualWhenInconsistent 两次裁决不一致：按不可重复处理，降级人工确认。
func TestReviewNeedsManualWhenInconsistent(t *testing.T) {
	env := newEnv(t)
	seedReviewPair(t, env)
	judge := &fakeJudge{winners: []string{ai.WinnerB, ai.WinnerA}}
	if n := runWorker(t, env, judge); n != 1 {
		t.Fatalf("worker 处理 %d 条，期望 1 条", n)
	}
	item := singleManual(t, env)
	if !strings.Contains(item.AI.Reasons[0], "不一致") {
		t.Errorf("降级原因 = %q，期望说明两次裁决不一致", item.AI.Reasons[0])
	}
}

// TestReviewKeepAndStateMachine keep 只改状态；非法迁移一律 409。
func TestReviewKeepAndStateMachine(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	runWorker(t, env, &fakeJudge{winners: []string{ai.WinnerB}})

	kept, code := decide(t, env, rev.ID, store.ReviewActionKeep)
	if code != http.StatusOK {
		t.Fatalf("keep 状态码 = %d，期望 200", code)
	}
	assertKept(t, env, rev, kept)
	assertIllegalTransitions(t, env, rev.ID)
}

// assertKept 断言 keep 的语义：只改状态，书侧完全不动，书架仍是旧书。
func assertKept(t *testing.T, env *testEnv, rev reviewItem, kept reviewItem) {
	t.Helper()
	if kept.Status != store.ReviewStatusConfirmedKeep {
		t.Errorf("keep 后状态 = %q，期望 confirmed_keep", kept.Status)
	}
	if kept.OldBook.SupersededBy != 0 || kept.NewBook.SupersededBy != rev.OldBook.ID {
		t.Errorf("keep 后 supersededBy = 旧 %d / 新 %d，期望书侧不变",
			kept.OldBook.SupersededBy, kept.NewBook.SupersededBy)
	}
	assertOnlyBook(t, env, rev.OldBook.ID)
}

// assertIllegalTransitions 断言已决策记录上的再决策 / 回滚一律 409 review_state。
func assertIllegalTransitions(t *testing.T, env *testEnv, id int64) {
	t.Helper()
	rr := env.do(http.MethodPost, fmt.Sprintf("/api/import/reviews/%d/decide", id),
		strings.NewReader(decideBody(store.ReviewActionReplace)),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusConflict, codeReviewState)
	rr = env.do(http.MethodPost, fmt.Sprintf("/api/import/reviews/%d/rollback", id), nil, nil)
	requireError(t, rr, http.StatusConflict, codeReviewState)
	// 不存在的记录 404，非法 action 400，非法 id 400。
	rr = env.do(http.MethodPost, "/api/import/reviews/999999/decide",
		strings.NewReader(decideBody(store.ReviewActionKeep)),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusNotFound, codeNotFound)
	rr = env.do(http.MethodPost, fmt.Sprintf("/api/import/reviews/%d/decide", id),
		strings.NewReader(decideBody("bogus")),
		map[string]string{"Content-Type": "application/json"})
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	requireError(t, env.do(http.MethodPost, "/api/import/reviews/abc/decide", nil, nil),
		http.StatusBadRequest, codeInvalidID)
}

// TestImportWithoutAIKeyUnaffected 对照：没有 key、开关关闭时导入完全不受影响。
//
// 断言四件事：书架只有旧书、aiEnabled=false、审核记录停在 pending（等人工）、
// 两个导入任务都是 completed（AI 完全不在导入链路上）。
func TestImportWithoutAIKeyUnaffected(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	assertOnlyBook(t, env, rev.OldBook.ID)

	list := listReviews(t, env, "all")
	if list.AIEnabled {
		t.Errorf("aiEnabled = true，期望默认关闭")
	}
	if list.Items[0].Status != store.ReviewStatusPending {
		t.Errorf("未启动 worker 时状态 = %q，期望 pending", list.Items[0].Status)
	}
	assertJobsCompleted(t, env, 2)
}

// assertJobsCompleted 断言最近 count 个导入任务都是 completed 且各导入 1 本。
func assertJobsCompleted(t *testing.T, env *testEnv, count int) {
	t.Helper()
	jobs := decodeBody[struct {
		Items []store.ImportJob `json:"items"`
	}](t, env.get("/api/import/jobs"))
	if len(jobs.Items) != count {
		t.Fatalf("导入任务 = %d 条，期望 %d 条", len(jobs.Items), count)
	}
	for _, job := range jobs.Items {
		if job.Status != "completed" || job.Imported != 1 {
			t.Errorf("任务 %d = %s / 导入 %d，期望 completed / 1", job.ID, job.Status, job.Imported)
		}
	}
}

// TestReviewWorkerBackground 后台 worker 会自己把 pending 处理掉（覆盖 run 循环与真实开关）。
func TestReviewWorkerBackground(t *testing.T) {
	env := newEnv(t)
	seedReviewPair(t, env)
	// 走真实开关：settings 的 ai.enabled 打开（默认关闭）。
	if err := env.st.PutSetting(context.Background(), store.SettingKeyAIEnabled, "true"); err != nil {
		t.Fatalf("打开 AI 开关: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartReviewWorker(ctx, env.st, &fakeJudge{winners: []string{ai.WinnerB}})
	waitForReady(t, env)
}

// waitForReady 轮询到出现 ready 记录（最多 3 秒）。
func waitForReady(t *testing.T, env *testEnv) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(listReviews(t, env, "ready").Items) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("后台 worker 没有在 3s 内把 pending 处理成 ready")
}

// TestReviewListQuery 列表的参数与空态：limit 越界 400、空列表是 []。
func TestReviewListQuery(t *testing.T) {
	env := newEnv(t)
	requireError(t, env.get("/api/import/reviews?limit=0"), http.StatusBadRequest, codeInvalidParam)
	empty := listReviews(t, env, "")
	if empty.Items == nil || len(empty.Items) != 0 {
		t.Errorf("空列表 = %+v，期望空数组（不是 null）", empty.Items)
	}
	if empty.AIEnabled {
		t.Errorf("aiEnabled = true，期望默认 false")
	}
}
