package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"norvels/internal/ai"
	"norvels/internal/store"
)

// 本文件是 AI 审核（0021，设计 §6 / §7）端到端测试的公共夹具：合成语料 → 真实导入链路
// 产生审核记录 → fake Judge 裁决 → replace / rollback 在书架上的效果。
//
// 语料全是造出来的（虚构书名 + 合成正文），不碰真实语料与真实书名；AI 用替身注入，
// 不需要真 key，也不出网。worker 侧的降级 / keep / 409 见 reviews_worker_test.go。

// reviewChapterCount 是版本对照的章数：24 章里有 1 章不同，字节加权命中率 23/24 ≈ 0.958，
// 仍在 0.95 阈值之上（不卡边界）。
const reviewChapterCount = 24

// reviewNovel 造一本合成小说：dirty 为真时每章后插一行广告（清洗器整行丢弃 → 噪声率），
// 并在 brokenChapter 那一章尾部加一串 U+FFFD（→ 乱码率，且该章指纹与干净版不同）。
func reviewNovel(dirty bool, brokenChapter int) string {
	var b strings.Builder
	b.WriteString("书名：示例书\n作者：示例作者\n\n")
	for i := 0; i < reviewChapterCount; i++ {
		fmt.Fprintf(&b, "第%d章 风雪%d\n", i+1, i)
		b.WriteString(strings.Repeat(fmt.Sprintf("梅宁城第%d次下雪，少年按住剑柄，远处的雪线压得很低。\n", i), 12))
		if i == brokenChapter-1 {
			b.WriteString("乱码\uFFFD\uFFFD\uFFFD\uFFFD\n")
		}
		if dirty {
			b.WriteString("本作品来自互联网，请支持正版。\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// fakeJudge 是 ai.Judge 的替身：按 winners 依次返回，并记录收到的请求（隐私断言用）。
type fakeJudge struct {
	mu      sync.Mutex
	winners []string
	err     error
	reqs    []ai.Request
}

// Judge 实现 ai.Judge。
func (f *fakeJudge) Judge(_ context.Context, req ai.Request) (ai.Verdict, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return ai.Verdict{}, f.err
	}
	winner := ai.WinnerB
	if len(f.winners) > 0 {
		winner = f.winners[0]
		if len(f.winners) > 1 {
			f.winners = f.winners[1:]
		}
	}
	return ai.Verdict{Winner: winner, Confidence: 0.82, Reasons: []string{"错字更少", "广告已清除"}}, nil
}

// importBody 生成导入请求体（用 json.Marshal 而不是手写转义）。
func importBody(dir string) string {
	raw, _ := json.Marshal(importRequest{Dir: dir})
	return string(raw)
}

// decideBody 生成决策请求体。
func decideBody(action string) string {
	raw, _ := json.Marshal(reviewDecideRequest{Action: action})
	return string(raw)
}

// importTxt 通过 POST /api/import 导入一个 txt 文件并等任务结束。
func importTxt(t *testing.T, env *testEnv, name, text string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("写入 %s: %v", name, err)
	}
	rr := env.do(http.MethodPost, "/api/import", strings.NewReader(importBody(path)),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("导入 %s 状态码 = %d，body=%s", name, rr.Code, rr.Body.String())
	}
	job := decodeBody[struct {
		JobID int64 `json:"jobId"`
	}](t, rr)
	if detail := env.waitJobDone(job.JobID, 10*time.Second); detail == nil || detail.Status != "completed" {
		t.Fatalf("导入任务 %d 未正常完成：%+v", job.JobID, detail)
	}
}

// seedReviewPair 导入「旧版脏 / 新版干净」两本书，返回列表里的那条审核记录。
func seedReviewPair(t *testing.T, env *testEnv) reviewItem {
	t.Helper()
	// 旧版：每章带广告行 + 第 7 章带乱码；新版：两者都没有（同一本书的干净重制版）。
	importTxt(t, env, "旧版.txt", reviewNovel(true, 7))
	importTxt(t, env, "新版.txt", reviewNovel(false, 0))
	list := listReviews(t, env, "all")
	if len(list.Items) != 1 {
		t.Fatalf("审核记录 = %d 条，期望 1 条（升级候选）：%+v", len(list.Items), list.Items)
	}
	return list.Items[0]
}

// listReviews 调 GET /api/import/reviews。
func listReviews(t *testing.T, env *testEnv, status string) reviewListResponse {
	t.Helper()
	rr := env.get("/api/import/reviews?status=" + status)
	if rr.Code != http.StatusOK {
		t.Fatalf("审核列表状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	return decodeBody[reviewListResponse](t, rr)
}

// runWorker 跑一轮 worker（注入替身裁决器），返回处理的条数。
func runWorker(t *testing.T, env *testEnv, judge ai.Judge) int {
	t.Helper()
	w := &reviewWorker{
		st: env.st, judge: judge,
		logf:    func(string, ...any) {},
		enabled: func(context.Context) bool { return true },
	}
	return w.ProcessPending(context.Background())
}

// decide 调决策端点并返回记录与状态码。
func decide(t *testing.T, env *testEnv, id int64, action string) (reviewItem, int) {
	t.Helper()
	rr := env.do(http.MethodPost, fmt.Sprintf("/api/import/reviews/%d/decide", id),
		strings.NewReader(decideBody(action)),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusOK {
		return reviewItem{}, rr.Code
	}
	return decodeBody[reviewItem](t, rr), rr.Code
}

// reviewShelfIDs 返回书架（GET /api/books）里的书 id。
func reviewShelfIDs(t *testing.T, env *testEnv) []int64 {
	t.Helper()
	rr := env.get("/api/books")
	if rr.Code != http.StatusOK {
		t.Fatalf("书籍列表状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	page := decodeBody[struct {
		Items []store.BookSummary `json:"items"`
		Total int                 `json:"total"`
	}](t, rr)
	if page.Total != len(page.Items) {
		t.Fatalf("total = %d 与 items = %d 不一致", page.Total, len(page.Items))
	}
	out := make([]int64, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.ID)
	}
	return out
}

// assertOnlyBook 断言书架只剩这一本。
func assertOnlyBook(t *testing.T, env *testEnv, want int64) {
	t.Helper()
	if got := reviewShelfIDs(t, env); len(got) != 1 || got[0] != want {
		t.Fatalf("书架 = %v，期望只有 %d", got, want)
	}
}

// assertBookStatus 断言详情端点的状态码（隐藏的书 404）。
func assertBookStatus(t *testing.T, env *testEnv, bookID int64, want int) {
	t.Helper()
	if rr := env.get(fmt.Sprintf("/api/books/%d", bookID)); rr.Code != want {
		t.Errorf("书 %d 详情状态码 = %d，期望 %d", bookID, rr.Code, want)
	}
}

// TestReviewReplaceThenRollback pending → ready → replace → 书架只剩新书 → rollback 恢复。
func TestReviewReplaceThenRollback(t *testing.T) {
	env := newEnv(t)
	rev := seedReviewPair(t, env)
	assertPendingReview(t, env, rev)

	judge := &fakeJudge{winners: []string{ai.WinnerB}}
	if n := runWorker(t, env, judge); n != 1 {
		t.Fatalf("worker 处理 %d 条，期望 1 条", n)
	}
	assertJudgePrivacy(t, judge)
	assertReadyReview(t, env)

	decided, code := decide(t, env, rev.ID, store.ReviewActionReplace)
	if code != http.StatusOK {
		t.Fatalf("replace 状态码 = %d，期望 200", code)
	}
	assertReplaced(t, env, rev, decided)

	rr := env.do(http.MethodPost, fmt.Sprintf("/api/import/reviews/%d/rollback", rev.ID), nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("回滚状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	assertRolledBack(t, env, rev, decodeBody[reviewItem](t, rr))
}

// assertPendingReview 断言导入后的初始状态：pending、新书隐藏、书架只有旧书。
func assertPendingReview(t *testing.T, env *testEnv, rev reviewItem) {
	t.Helper()
	if rev.Status != store.ReviewStatusPending {
		t.Fatalf("导入后的审核状态 = %q，期望 pending", rev.Status)
	}
	if rev.NewBook.SupersededBy != rev.OldBook.ID {
		t.Errorf("新书 supersededBy = %d，期望 %d（导入即隐藏）", rev.NewBook.SupersededBy, rev.OldBook.ID)
	}
	assertOnlyBook(t, env, rev.OldBook.ID)
}

// assertJudgePrivacy 断言外发请求的形状：两次裁决、不带书名、只带差异正文。
func assertJudgePrivacy(t *testing.T, judge *fakeJudge) {
	t.Helper()
	if len(judge.reqs) != 2 {
		t.Fatalf("裁决调用 = %d 次，期望 2 次（两次一致性校验）", len(judge.reqs))
	}
	for _, req := range judge.reqs {
		if req.Title != "" {
			t.Errorf("请求带了标题 %q，隐私红线不允许", req.Title)
		}
		for _, seg := range req.Segments {
			text := seg.A + seg.B + seg.Context
			if strings.Contains(text, "旧版") || strings.Contains(text, "新版") {
				t.Errorf("外发正文里出现了书名：%+v", seg)
			}
		}
	}
}

// assertReadyReview 断言 AI 结论写回 ready（含置信度、理由、规则对比）。
func assertReadyReview(t *testing.T, env *testEnv) {
	t.Helper()
	ready := listReviews(t, env, "ready")
	if len(ready.Items) != 1 {
		t.Fatalf("ready 列表 = %d 条，期望 1 条", len(ready.Items))
	}
	item := ready.Items[0]
	if item.AI.Winner != store.ReviewWinnerNew || item.AI.Confidence != 0.82 || len(item.AI.Reasons) == 0 {
		t.Errorf("AI 结论 = %+v，期望 new / 0.82 / 带理由", item.AI)
	}
	if len(item.RuleReasons) < 2 {
		t.Errorf("规则对比 = %v，期望至少两项", item.RuleReasons)
	}
}

// assertReplaced 断言替换效果：两本书的 superseded 互换、书架只剩新书、旧书 404。
func assertReplaced(t *testing.T, env *testEnv, rev reviewItem, decided reviewItem) {
	t.Helper()
	if decided.Status != store.ReviewStatusConfirmedReplace {
		t.Errorf("替换后状态 = %q，期望 confirmed_replace", decided.Status)
	}
	if decided.NewBook.SupersededBy != 0 || decided.OldBook.SupersededBy != rev.NewBook.ID {
		t.Errorf("替换后 supersededBy = 旧 %d / 新 %d，期望旧→新、新→0",
			decided.OldBook.SupersededBy, decided.NewBook.SupersededBy)
	}
	assertOnlyBook(t, env, rev.NewBook.ID)
	assertBookStatus(t, env, rev.OldBook.ID, http.StatusNotFound)
	assertBookStatus(t, env, rev.NewBook.ID, http.StatusOK)
}

// assertRolledBack 断言回滚效果：旧书恢复可见，新书重新隐藏。
func assertRolledBack(t *testing.T, env *testEnv, rev reviewItem, rolled reviewItem) {
	t.Helper()
	if rolled.Status != store.ReviewStatusRolledBack {
		t.Errorf("回滚后状态 = %q，期望 rolled_back", rolled.Status)
	}
	assertOnlyBook(t, env, rev.OldBook.ID)
	assertBookStatus(t, env, rev.NewBook.ID, http.StatusNotFound)
}
