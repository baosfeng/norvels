package api

import (
	"context"

	"norvels/internal/recommend/recall"
	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 多路召回（设计 §3）：兴趣分类 / 兴趣作者 / 标签 / 系列 / 共读 / 热门 / 新鲜 / 随机 / 续读。
//
// 召回逻辑整体在 internal/recommend/recall：它同时读主库与推荐库，但不需要 Server
// 的其它能力。这里只剩两件事——把两个库适配成 recall 的 Port / RecPort，以及保持
// Server 原有的调用点（装配与打分仍在 handlers_recommend.go）。
//
// recentShownDays 是「近期已曝光」的窗口：7 天内推过的不重复推（只扣分不硬过滤）。
const recentShownDays = 7

// recallPort 把主库与推荐库适配成 recall 的查询接口。
type recallPort struct {
	st  *store.Store
	rec *recstore.Store
}

// ports 取当前 Server 的两个库适配器（同一个值同时充当 Port 与 RecPort）。
func (s *Server) ports() recallPort { return recallPort{st: s.st, rec: s.cfg.Rec} }

// 以下方法是 recall.Port / recall.RecPort 的薄适配：store 与 recstore 的方法签名
// 已与接口一致，这里逐字转发，不做任何类型转换或默认值补偿（否则行为会漂移）。

func (p recallPort) ListBooks(ctx context.Context, q store.BookQuery) (store.Page[store.BookSummary], error) {
	return p.st.ListBooks(ctx, q)
}

func (p recallPort) BooksByIDs(ctx context.Context, ids []int64) ([]store.BookSummary, error) {
	return p.st.BooksByIDs(ctx, ids)
}

func (p recallPort) BookTagsByIDs(ctx context.Context, ids []int64) (map[int64][]string, error) {
	return p.st.BookTagsByIDs(ctx, ids)
}

func (p recallPort) ListReadingProgress(ctx context.Context, limit int) ([]store.ReadingProgress, error) {
	return p.st.ListReadingProgress(ctx, limit)
}

func (p recallPort) RandomBookSummaries(ctx context.Context, n int) ([]store.BookSummary, error) {
	return p.st.RandomBookSummaries(ctx, n)
}

func (p recallPort) Cooccur(ctx context.Context, userID int64, limit int) ([]recstore.Cooccur, error) {
	return p.rec.Cooccur(ctx, userID, limit)
}

func (p recallPort) CooccurForBook(ctx context.Context, bookID int64, limit int) ([]recstore.Cooccur, error) {
	return p.rec.CooccurForBook(ctx, bookID, limit)
}

func (p recallPort) HotBooks(ctx context.Context, days, limit int) ([]recstore.HotBook, error) {
	return p.rec.HotBooks(ctx, days, limit)
}

// recallCandidates 按 tab 跑对应召回（tab 的合法性已在 HTTP 层校验）。
func (s *Server) recallCandidates(ctx context.Context, userID int64,
	interest map[string]float64, tab string) (*recall.Result, error) {
	ports := s.ports()
	return recall.Candidates(ctx, ports, ports, userID, interest, tab)
}

// recallSimilar 取与 bookID 相似的书（排除自身，书不存在时返回 ErrNotFound）。
func (s *Server) recallSimilar(ctx context.Context, bookID int64) (*recall.Result, error) {
	ports := s.ports()
	return recall.Similar(ctx, ports, ports, bookID)
}
