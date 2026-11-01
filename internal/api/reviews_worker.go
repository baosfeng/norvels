package api

import (
	"context"
	"errors"
	"log"
	"time"

	"norvels/internal/ai"
	"norvels/internal/store"
)

// 本文件是后台 AI 审核 worker（设计 §6）：异步、单线程、可关闭，绝不阻塞导入。
//
// # 生命周期
//
// 由 cmd 的 serve 在启动时调用 StartReviewWorker(ctx, st, judge)，进程收到退出信号
// （ctx 取消）后下一轮循环即退出。**不在 api.New 里自动启动**：构造一个 handler 不该
// 顺带起后台任务（测试与 CLI 子命令都会被牵连）。
//
// # 并发
//
// 单 goroutine 串行处理：AI 裁决是网络 + 两次一致性校验，并发只会把配额与账单放大，
// 而待审记录本来就很少（只有「升级候选」才产生）。写回走 store 的条件更新
// （WHERE status='pending'），即使用户在裁决期间先点了按钮，也不会被覆盖。
//
// # 降级
//
// 开关关闭 / 没有 key / 两版没有可比对的差异 / 超时 / 两次不一致 —— 一律落
// needs_manual（带一条人话原因），等人工判断。导入流程完全不知道这里发生过什么。

const (
	reviewWorkerInterval = 5 * time.Second   // 轮询间隔
	reviewWorkerBatch    = 5                 // 每轮最多处理几条
	reviewAITimeout      = 150 * time.Second // 单条审核的 AI 总超时（两次裁决 + 5xx 重试余量）
)

// reviewWorker 是 AI 审核的后台执行者。
type reviewWorker struct {
	st    *store.Store
	judge ai.Judge // nil = 没有配置 key，功能等同不存在
	logf  func(string, ...any)
	// enabled 报告 AI 开关（默认读 settings 的 ai.enabled）；测试可注入。
	enabled func(context.Context) bool
}

// StartReviewWorker 启动后台 worker（ctx 取消后它在下一次循环退出）。
//
// judge 为 nil 表示没有 key：worker 照样运行，把待审记录直接落 needs_manual，
// 让审核页能显示「AI 未启用，仅规则结论」，而不是让记录永远停在 pending。
func StartReviewWorker(ctx context.Context, st *store.Store, judge ai.Judge) *reviewWorker {
	w := &reviewWorker{st: st, judge: judge, logf: log.Printf, enabled: st.AIEnabled}
	go w.run(ctx)
	return w
}

// run 是 worker 主循环：先处理一轮（启动即处理历史 pending），之后按间隔轮询。
func (w *reviewWorker) run(ctx context.Context) {
	ticker := time.NewTicker(reviewWorkerInterval)
	defer ticker.Stop()
	for {
		w.ProcessPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessPending 处理一轮待审记录，返回本轮处理的条数（测试直接调它，不等 ticker）。
func (w *reviewWorker) ProcessPending(ctx context.Context) int {
	if w.st == nil {
		return 0
	}
	revs, err := w.st.PendingImportReviews(ctx, reviewWorkerBatch)
	if err != nil {
		w.logf("api: 读取待审核记录失败: %v", err)
		return 0
	}
	done := 0
	for _, rev := range revs {
		if ctx.Err() != nil {
			break
		}
		w.processOne(ctx, rev)
		done++
	}
	return done
}

// processOne 裁决一条记录并写回；写回失败只记日志（下一轮还会重新取到它）。
func (w *reviewWorker) processOne(ctx context.Context, rev store.ImportReview) {
	res := w.judgeOne(ctx, rev)
	if err := w.st.FinishImportReviewAI(ctx, rev.ID, res); err != nil {
		w.logf("api: 写回审核结果失败 review=%d: %v", rev.ID, err)
		return
	}
	w.logf("api: 审核 %d → %s（旧书 %d / 新书 %d，AI %s）",
		rev.ID, res.Status, rev.OldBookID, rev.NewBookID, res.Winner)
}

// judgeOne 组装输入并裁决；任何异常都翻成 needs_manual（本方法不返回 error）。
func (w *reviewWorker) judgeOne(ctx context.Context, rev store.ImportReview) store.ReviewAIResult {
	if !w.enabled(ctx) {
		return needsManual("AI 未启用（settings ai.enabled=false）")
	}
	if w.judge == nil {
		return needsManual("没有配置 API key（环境变量 NORVELS_AI_API_KEY）")
	}
	diff, err := w.st.SampleReviewDiff(ctx, rev.OldBookID, rev.NewBookID)
	if err != nil {
		return needsManual("读取两版正文失败：" + err.Error())
	}
	if len(diff.Segments) == 0 {
		return needsManual("两版正文没有可比对的差异段落")
	}
	verdict, err := w.judgeTwice(ctx, diff.Segments)
	if err != nil {
		return needsManual("AI 裁决不可用：" + aiReason(err))
	}
	return store.ReviewAIResult{
		Status:     store.ReviewStatusReady,
		Winner:     winnerName(verdict.Winner),
		Confidence: verdict.Confidence,
		Reasons:    verdict.Reasons,
	}
}

// judgeTwice 调两次裁决并做一致性校验（ai.JudgeTwice），带总超时。
//
// 请求里**不带书名与作者**：ai.Request.Title 只给调用方打日志用，这里留空；
// 外发的只有差异段落正文与上下文（隐私红线，见 internal/ai 包注释）。
func (w *reviewWorker) judgeTwice(ctx context.Context, segs []store.ReviewSegment) (ai.Verdict, error) {
	actx, cancel := context.WithTimeout(ctx, reviewAITimeout)
	defer cancel()
	return ai.JudgeTwice(actx, w.judge, ai.Request{Segments: toAISegments(segs)})
}

// toAISegments 把 store 的片段翻成 ai.Segment（A = 旧版、B = 新版，调用方约定）。
func toAISegments(segs []store.ReviewSegment) []ai.Segment {
	out := make([]ai.Segment, 0, len(segs))
	for _, s := range segs {
		out = append(out, ai.Segment{Context: s.Context, A: s.Old, B: s.New})
	}
	return out
}

// winnerName 把 ai 包的呈现标签翻成落库语义值（old / new / equal）。
func winnerName(winner string) string {
	switch winner {
	case ai.WinnerA:
		return store.ReviewWinnerOld
	case ai.WinnerB:
		return store.ReviewWinnerNew
	default:
		return store.ReviewWinnerEqual
	}
}

// needsManual 构造一条降级结果（原因进 ai_reasons，前端直接展示）。
func needsManual(reason string) store.ReviewAIResult {
	return store.ReviewAIResult{Status: store.ReviewStatusNeedsManual, Reasons: []string{reason}}
}

// aiReason 把 ai 包的分类错误翻成一句人话（未分类的错误原样带出）。
func aiReason(err error) string {
	switch {
	case errors.Is(err, ai.ErrNoKey):
		return "没有配置 API key"
	case errors.Is(err, ai.ErrNoSegments):
		return "没有可裁决的差异段落"
	case errors.Is(err, ai.ErrTimeout):
		return "请求超时"
	case errors.Is(err, ai.ErrServer):
		return "服务端错误（5xx）"
	case errors.Is(err, ai.ErrClient):
		return "请求被拒绝（4xx，通常是 key 或模型名）"
	case errors.Is(err, ai.ErrNetwork):
		return "网络错误"
	case errors.Is(err, ai.ErrBadResponse):
		return "响应不是合法的裁决 JSON"
	case errors.Is(err, ai.ErrInconsistent):
		return "两次裁决结果不一致"
	}
	return err.Error()
}
