package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"norvels/internal/crawl"
	"norvels/internal/enrich"
	"norvels/internal/model"
	"norvels/internal/store"
)

// titleEchoEngine 把书名原样当作命中书名返回：相似度恒为 1，
// 每本书都必然产出变更，flush 批次的条数只取决于书数（可精确断言）。
type titleEchoEngine struct {
	mu       sync.Mutex
	requests int
}

func (e *titleEchoEngine) SearchBook(_ context.Context, title string) (crawl.Result, error) {
	e.mu.Lock()
	e.requests++
	e.mu.Unlock()
	return crawl.Result{Hits: []enrich.Hit{echoHit(title)}}, nil
}

func (e *titleEchoEngine) Stats() crawl.Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return crawl.Stats{Requests: e.requests}
}

// echoHit 造一条与候选同名的命中（作者/简介/分类三个字段都待补）。
func echoHit(title string) enrich.Hit {
	return enrich.Hit{
		SourceID: "src-a", Priority: 1, Title: title, Author: "作者甲",
		Intro: "这是一段足够长的示例简介，用来验证简介字段能够参与决策并写进变更清单。",
		Kind:  "玄幻/东方玄幻",
	}
}

// seedManyCandidateBooks 造 n 本待补书（书名/指纹各不相同，避免导入判重）。
func seedManyCandidateBooks(t *testing.T, st *store.Store, n int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		book := &model.ParsedBook{
			Title:      fmt.Sprintf("示例书名第%d册", i+1),
			Author:     "佚名",
			SourcePath: fmt.Sprintf("/books/demo-%02d.txt", i),
			SourceName: fmt.Sprintf("demo-%02d.txt", i),
			Encoding:   "utf-8", FileSize: 1024,
			SHA256:    fmt.Sprintf("%064d", i+1),
			WordCount: 30,
			Chapters: []model.ParsedChapter{
				{Idx: 0, VolumeIdx: -1, Title: "第一章", Content: "示例正文。", WordCount: 5},
			},
		}
		res, err := st.SaveBook(t.Context(), book, store.SaveOptions{})
		if err != nil {
			t.Fatalf("种子书 %d: %v", i, err)
		}
		ids = append(ids, res.BookID)
	}
	return ids
}

// listCandidates 把种子书转成候选清单（不查库，避免依赖 --fields 口径）。
func listCandidates(ids []int64) []store.EnrichCandidate {
	out := make([]store.EnrichCandidate, 0, len(ids))
	for i, id := range ids {
		out = append(out, store.EnrichCandidate{
			BookID: id, Title: fmt.Sprintf("示例书名第%d册", i+1), Author: "佚名",
		})
	}
	return out
}

// flushCall 记录一次 flush 的规模与「当时库里已记录的条数」。
type flushCall struct {
	changes int
	inDB    int
	err     error
}

// recordingFlush 包一层真实落库，记录每次 flush 的规模与调用时库里的行数，
// 用来证明「循环中途就已经落库」，而不是全部跑完才写一次。
func recordingFlush(t *testing.T, st *store.Store, batchID string, calls *[]flushCall) flushFunc {
	t.Helper()
	base := batchFlusher(st, batchID)
	return func(ctx context.Context, changes []store.EnrichChange) (int, error) {
		written, err := base(ctx, changes)
		inDB := 0
		if rows, qerr := st.EnrichBatchChanges(ctx, batchID); qerr == nil {
			inDB = len(rows)
		}
		*calls = append(*calls, flushCall{changes: len(changes), inDB: inDB, err: err})
		return written, err
	}
}

// runStream 跑一次流式循环（不落到报告打印），返回报告与进度输出。
func runStream(t *testing.T, st *store.Store, cands []store.EnrichCandidate,
	o enrichOpts, sink streamSink, out *bytes.Buffer) enrichReport {
	t.Helper()
	sink.Out = out
	o.fields = map[string]bool{
		store.EnrichFieldAuthor: true, store.EnrichFieldIntro: true, store.EnrichFieldCategory: true,
	}
	rep := runEnrichStreamTo(t.Context(), st, &titleEchoEngine{}, cands, o, sink)
	rep.BatchID = sink.BatchID
	return rep
}

// TestStreamFlushesMidLoop -flush-every 2 + 5 本：循环中途 enrich_changes 就有行。
func TestStreamFlushesMidLoop(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 5)
	calls := []flushCall{}
	const batchID = "20260101-000000-abcdef"
	var out bytes.Buffer
	rep := runStream(t, st, listCandidates(ids),
		enrichOpts{flushEvery: 2, workers: 2},
		streamSink{Flush: recordingFlush(t, st, batchID, &calls), BatchID: batchID}, &out)

	if len(calls) != 3 {
		t.Fatalf("5 本 / 每 2 本 flush = 3 次（含尾巴），实际 %d 次：%+v", len(calls), calls)
	}
	// 第一次 flush 发生在前 2 本处理完时，此时库里必须已经有行——
	// 这正是「中途零落库」被修掉的断言点。
	if calls[0].changes == 0 || calls[0].inDB != calls[0].changes {
		t.Fatalf("第一次 flush 后库里应有 %d 行，实际 %+v", calls[0].changes, calls[0])
	}
	if rows, err := st.EnrichBatchChanges(t.Context(), batchID); err != nil || len(rows) != 15 {
		t.Fatalf("结束时库里应有 15 条（5 本 × 3 字段），实际 %d 条 / err=%v", len(rows), err)
	}
	if rep.TotalChanges != 15 || rep.changeCount() != 15 {
		t.Fatalf("报告变更总数应为 15，实际 TotalChanges=%d changeCount=%d", rep.TotalChanges, rep.changeCount())
	}
	if rep.Written != 15 || rep.Failed != 0 {
		t.Fatalf("应写库 15 条且无失败，实际 written=%d failed=%d", rep.Written, rep.Failed)
	}
}

// TestStreamTailFlushCoversRemainder 5 本 / 每 2 本：尾巴（第 5 本）由结束时的 flush 覆盖。
func TestStreamTailFlushCoversRemainder(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 5)
	calls := []flushCall{}
	const batchID = "20260101-000000-tail01"
	var out bytes.Buffer
	runStream(t, st, listCandidates(ids), enrichOpts{flushEvery: 2, workers: 1},
		streamSink{Flush: recordingFlush(t, st, batchID, &calls), BatchID: batchID}, &out)

	last := calls[len(calls)-1]
	if last.changes == 0 {
		t.Fatalf("最后一批应是剩余不足 N 本的尾巴，实际 %+v", calls)
	}
	total := 0
	for _, c := range calls {
		total += c.changes
	}
	if total != 15 || last.changes != 3 {
		t.Fatalf("三次 flush 应为 6+6+3=15 条，实际合计 %d，末批 %d：%+v", total, last.changes, calls)
	}
}

// TestStreamFlushFailureRecorded flush 注入错误：不 panic，报告里能看出失败条数且不写数据。
func TestStreamFlushFailureRecorded(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 4)
	var out bytes.Buffer
	boom := errors.New("注入的写库故障")
	rep := runStream(t, st, listCandidates(ids), enrichOpts{flushEvery: 2, workers: 1},
		streamSink{Flush: func(context.Context, []store.EnrichChange) (int, error) {
			return 0, boom
		}, BatchID: "20260101-000000-fail01"}, &out)

	if rep.Failed != rep.TotalChanges || rep.TotalChanges == 0 {
		t.Fatalf("失败条数应等于变更总数 %d，实际 failed=%d", rep.TotalChanges, rep.Failed)
	}
	if rep.Written != 0 {
		t.Fatalf("记 intent 失败时不该有写成功的条数，实际 %d", rep.Written)
	}
	if rows, _ := st.EnrichBatchChanges(t.Context(), "20260101-000000-fail01"); len(rows) != 0 {
		t.Fatalf("注入失败时库里不该有行，实际 %d 条", len(rows))
	}
	for _, id := range ids {
		detail, err := st.GetBook(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Authors) > 0 && detail.Authors[0] == "作者甲" {
			t.Fatalf("记 intent 失败的书 %d 不应被改数据", id)
		}
	}
}

// TestStreamSingleBatchID 整批共用一个 batch_id（一次 --rollback 能回滚整批）。
func TestStreamSingleBatchID(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 5)
	const batchID = "20260101-000000-same01"
	var out bytes.Buffer
	rep := runStream(t, st, listCandidates(ids), enrichOpts{flushEvery: 2, workers: 2},
		streamSink{Flush: batchFlusher(st, batchID), BatchID: batchID}, &out)

	if rep.BatchID != batchID {
		t.Fatalf("报告批次号应为 %q，实际 %q", batchID, rep.BatchID)
	}
	rows, err := st.EnrichBatchChanges(t.Context(), batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 15 {
		t.Fatalf("同一批次应累计 15 条，实际 %d 条", len(rows))
	}
	if _, err := st.RollbackEnrichBatch(t.Context(), batchID); err != nil {
		t.Fatalf("整批应能一次回滚，实际 %v", err)
	}
}

// TestStreamProgressLogged 进度日志至少触发一次，且带上已处理/总数与已写库条数。
func TestStreamProgressLogged(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 5)
	var out bytes.Buffer
	runStream(t, st, listCandidates(ids), enrichOpts{flushEvery: 2, workers: 1},
		streamSink{Flush: batchFlusher(st, "20260101-000000-prog01"), BatchID: "20260101-000000-prog01"},
		&out)

	progress := out.String()
	if !strings.Contains(progress, "进度：") {
		t.Fatalf("应打印进度行，实际输出 %q", progress)
	}
	for _, want := range []string{"2/5 本", "已写库 6 条", "请求"} {
		if !strings.Contains(progress, want) {
			t.Fatalf("进度行应包含 %q，实际 %q", want, progress)
		}
	}
}

// TestStreamFlushZeroOnlyAtEnd -flush-every 0：不中途落库，只在结束时 flush 一次。
func TestStreamFlushZeroOnlyAtEnd(t *testing.T) {
	st := openTempStore(t)
	defer st.Close()
	ids := seedManyCandidateBooks(t, st, 5)
	calls := []flushCall{}
	const batchID = "20260101-000000-zero01"
	var out bytes.Buffer
	runStream(t, st, listCandidates(ids), enrichOpts{flushEvery: 0, workers: 2},
		streamSink{Flush: recordingFlush(t, st, batchID, &calls), BatchID: batchID}, &out)

	if len(calls) != 1 || calls[0].changes != 15 {
		t.Fatalf("--flush-every 0 应只有结束时一次 flush（15 条），实际 %+v", calls)
	}
	if strings.Contains(out.String(), "进度：") {
		t.Fatalf("flushEvery=0 且未到时间阈值时不该打印进度，实际 %q", out.String())
	}
}

// TestProgressTickerDue 覆盖节流规则：满 N 本必打、不满 N 本靠 30 秒兜底。
func TestProgressTickerDue(t *testing.T) {
	now := time.Now()
	ticker := newProgressTicker(2, now)
	if !ticker.due(2) {
		t.Fatal("满 N 本应打印进度")
	}
	if ticker.due(3) {
		t.Fatal("未满 N 本且未超时不该打印进度")
	}
	slow := newProgressTicker(2, now.Add(-progressInterval))
	if !slow.due(3) {
		t.Fatal("距上次超过 30 秒应打印进度")
	}
	zero := newProgressTicker(0, now.Add(-progressInterval))
	if !zero.due(1) {
		t.Fatal("flushEvery=0 时按时间兜底打印进度")
	}
}
