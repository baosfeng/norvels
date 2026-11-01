package api

import (
	"context"
	"errors"
	"net/http"

	"norvels/internal/recstore"
	"norvels/internal/store"
)

// 埋点上报（设计 §4）：POST /api/events。
//
// 前端 SDK 把 1 秒内的曝光 / 点击合并成一批发上来；服务端只做「校验 + 转成 rec 事件」，
// 聚合口径全部在 recstore 里——只有那一层知道 impression 只进聚合、不落明细。

const (
	// maxEventBatch / maxEventFieldLen / maxEventExtraLen 与 recstore 的同名限制同值：
	// api 先拦成 400（人能看懂「第几条错了」），数据层再兜一次。跨信任边界两道门，
	// 口径必须一致，否则会出现「api 放行、数据层拒绝」的 500。
	maxEventBatch    = recstore.MaxEventBatch
	maxEventFieldLen = 64
	maxEventExtraLen = 512
	// maxEventTags 与 recstore.maxTagsPerEvent 同值（api 先拦成带下标的 400）。
	maxEventTags = 20
)

// eventsRequest 是 POST /api/events 的请求体。
type eventsRequest struct {
	Events []eventPayload `json:"events"`
}

// eventPayload 是一条埋点（字段名与前端 telemetry SDK 一致）。
type eventPayload struct {
	Kind       string   `json:"kind"`
	BookID     int64    `json:"bookId"`
	AuthorID   int64    `json:"authorId"`
	CategoryL1 string   `json:"categoryL1"`
	CategoryL2 string   `json:"categoryL2"`
	Tags       []string `json:"tags"`
	Series     string   `json:"series"`
	Value      int      `json:"value"`
	Extra      string   `json:"extra"`
	TS         string   `json:"ts"`
}

// handleRecordEvents 处理 POST /api/events：批量写入推荐库，返回 {accepted}。
func (s *Server) handleRecordEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireRec(w) {
		return
	}
	var body eventsRequest
	if !decodeJSONBody(w, r, &body, "请求体必须是 {events:[...]} 对象") {
		return
	}
	if err := validateEventBatch(body.Events); err != nil {
		s.badRequest(w, err, codeInvalidParam)
		return
	}
	events := s.enrichEvents(r.Context(), toRecEvents(body.Events))
	accepted, err := s.cfg.Rec.RecordEvents(r.Context(), store.ScopeUserID(r.Context()), events)
	if err != nil {
		s.failRec(w, r, err, "事件写入失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"accepted": accepted})
}

// validateEventBatch 校验整批事件：条数上限 + 逐条字段。
//
// 整批拒绝而不是跳过坏条目：批量上报是前端的重试单位，混合接受会让客户端
// 无法判断该重发哪一条，最后整批重发并造成翻倍写入。
func validateEventBatch(events []eventPayload) error {
	switch {
	case len(events) == 0:
		return badParam("events 不能为空")
	case len(events) > maxEventBatch:
		return badParam("单次最多 %d 条事件", maxEventBatch)
	}
	for i, ev := range events {
		if err := validateEvent(ev); err != nil {
			return badParam("第 %d 条：%s", i+1, err.Error())
		}
	}
	return nil
}

// validateEvent 校验单条事件的 kind / 必填字段 / 长度上限。
func validateEvent(ev eventPayload) error {
	if !validEventKind(ev.Kind) {
		return badParam("kind %q 不合法（impression|click|read|view|search|shelf|rating）", ev.Kind)
	}
	if ev.Kind != recstore.KindSearch && ev.BookID <= 0 {
		return badParam("%s 事件缺少 bookId", ev.Kind)
	}
	if len(ev.CategoryL1) > maxEventFieldLen || len(ev.CategoryL2) > maxEventFieldLen {
		return badParam("分类名超过 %d 字节", maxEventFieldLen)
	}
	if len(ev.Extra) > maxEventExtraLen {
		return badParam("extra 超过 %d 字节", maxEventExtraLen)
	}
	return validateEventTags(ev)
}

// validateEventTags 校验系列名与标签（数量 / 单个长度）。
//
// 单独拆出来是为了让 validateEvent 不突破门禁 4 的圈复杂度上限：
// 标签与系列都是「多值字段」，校验本身比其它字段多一层循环。
func validateEventTags(ev eventPayload) error {
	if len(ev.Series) > maxEventFieldLen {
		return badParam("系列名超过 %d 字节", maxEventFieldLen)
	}
	if len(ev.Tags) > maxEventTags {
		return badParam("单条事件最多 %d 个标签", maxEventTags)
	}
	for _, tag := range ev.Tags {
		if len(tag) > maxEventFieldLen {
			return badParam("标签名超过 %d 字节", maxEventFieldLen)
		}
	}
	return nil
}

// validEventKind 是 kind 白名单（与 recstore 的单一口径一致）。
func validEventKind(kind string) bool {
	switch kind {
	case recstore.KindImpression, recstore.KindClick, recstore.KindRead,
		recstore.KindView, recstore.KindSearch, recstore.KindShelf, recstore.KindRating,
		recstore.KindDislike:
		return true
	}
	return false
}

// toRecEvents 把 API 传输结构转成数据层事件（字段一一对应，不改语义）。
func toRecEvents(payloads []eventPayload) []recstore.Event {
	out := make([]recstore.Event, len(payloads))
	for i, p := range payloads {
		out[i] = recstore.Event{
			Kind: p.Kind, BookID: p.BookID, AuthorID: p.AuthorID,
			CategoryL1: p.CategoryL1, CategoryL2: p.CategoryL2,
			Tags: p.Tags, Series: p.Series,
			Value: p.Value, Extra: p.Extra, TS: p.TS,
		}
	}
	return out
}

// enrichEvents 用主库补齐埋点缺失的作者与分类（前端只保证带 bookId）。
//
// 为什么在服务端补：前端埋点只知道「点了哪本书」，作者与分类不一定在卡片数据里；
// 让每处调用各查一遍，同一本书的维度就会有的点带、有的点不带，兴趣权重也就
// 聚不到同一个 key 上（"category_l1:武侠" 与空 key 各拿一半权重，谁都进不了 top）。
// 一次 BooksByIDs 批量补齐，代价是一次按主键的查询。
func (s *Server) enrichEvents(ctx context.Context, events []recstore.Event) []recstore.Event {
	ids := missingDimsIDs(events)
	if len(ids) == 0 {
		return events
	}
	books, err := s.st.BooksByIDs(ctx, ids)
	if err != nil {
		// 补齐失败只降级：事件本身合法，客户端不该为「服务端查书失败」重发。
		s.logf("api: 补齐埋点维度时取书失败: %v", err)
		return events
	}
	byID := make(map[int64]store.BookSummary, len(books))
	for _, bs := range books {
		byID[bs.ID] = bs
	}
	tags, err := s.st.BookTagsByIDs(ctx, ids)
	if err != nil {
		s.logf("api: 补齐埋点标签失败（标签兴趣本次缺省）: %v", err)
		tags = nil
	}
	for i := range events {
		ev := &events[i]
		fillEventDims(ev, byID[ev.BookID], tags[ev.BookID])
	}
	return events
}

// missingDimsIDs 列出需要补齐维度的书 id。
//
// impression 不参与：曝光只进聚合（book_stats / book_daily / recent_items），
// 而曝光是量最大的一路，为它做一次查书换不来任何信号，纯属浪费。
func missingDimsIDs(events []recstore.Event) []int64 {
	out := make([]int64, 0, len(events))
	for _, ev := range events {
		if ev.Kind == recstore.KindImpression {
			continue
		}
		if ev.BookID > 0 && (ev.AuthorID == 0 || ev.CategoryL1 == "" || ev.CategoryL2 == "" ||
			ev.Series == "" || len(ev.Tags) == 0) {
			out = append(out, ev.BookID)
		}
	}
	return out
}

// fillEventDims 只补空字段，不覆盖客户端已经填好的维度。
func fillEventDims(ev *recstore.Event, bs store.BookSummary, tags []string) {
	if bs.ID <= 0 {
		return // 书已被删除：保持原样，让事件照常归档（兴趣维度留空）
	}
	if ev.AuthorID == 0 {
		ev.AuthorID = bs.AuthorID
	}
	if ev.CategoryL1 == "" {
		ev.CategoryL1 = bs.CategoryL1
	}
	if ev.CategoryL2 == "" {
		ev.CategoryL2 = bs.CategoryL2
	}
	if ev.Series == "" {
		ev.Series = bs.Series
	}
	if len(ev.Tags) == 0 {
		ev.Tags = tags
	}
}

// requireRec 确认推荐库已启用；未启用时写 503 并返回 false。
func (s *Server) requireRec(w http.ResponseWriter) bool {
	if s.cfg.Rec != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, codeUnavailable, "推荐功能未启用（推荐库不可用）")
	return false
}

// failRec 把推荐库错误翻成 HTTP：入参非法 → 400，其余 → 500。
//
// 「推荐库可重建」不等于可以吞错误：写失败时前端要知道这批埋点没记上，
// 否则「推荐怎么一直不变」会被当成算法问题排查。
func (s *Server) failRec(w http.ResponseWriter, r *http.Request, err error, msg string) {
	if errors.Is(err, recstore.ErrInvalidArgument) {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "请求参数非法")
		return
	}
	s.logf("api: %s %s 推荐库错误: %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusInternalServerError, codeInternal, msg)
}
