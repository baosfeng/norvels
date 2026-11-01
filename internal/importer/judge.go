package importer

// 上架判定端口（AI 选书）。
//
// importer 不认识任何具体判定实现：它只在一个确定的时点（章节门槛之后、写库之前）
// 把解析好的书交出去，问一句「导不导」。实现由调用方注入（cmd 里接 internal/ai），
// 因此 importer 的依赖白名单不用动，internal/ai 也不会反过来依赖导入链路。

import (
	"context"
	"fmt"

	"norvels/internal/model"
	"norvels/internal/store"
)

// Verdict 是一次上架判定的结论。
type Verdict struct {
	// Import 为 false 表示这本书不进库（判定器给了理由）。
	Import bool
	// Reason 是进报告与文件终态的一句话。
	Reason string
}

// Judge 判定一本书值不值得入库。
//
// 返回错误表示「判定不可用」。调用方按 fail-closed 处理：不入库并记原因。
// 开了外部判定却因为网络抖动把书悄悄导进去，等于这个开关没生效；
// 反过来说，判定不可用时宁可少导一本，也不要多导一本——书可以再导，错导要人工删。
type Judge func(ctx context.Context, book *model.ParsedBook) (Verdict, error)

// decide 跑一次外部判定；未注入 Judge 时恒为「导」。
func (r *runState) decide(ctx context.Context, oc parseOutcome, book *model.ParsedBook, opt Options) bool {
	if opt.Judge == nil {
		return true
	}
	v, err := opt.Judge(ctx, book)
	if err != nil {
		r.res.Skipped++
		r.recordJobFile(ctx, oc.cand.source, oc.path, store.ImportFileStatusSkipped,
			fmt.Sprintf("上架判定不可用：%v", err), 0)
		return false
	}
	if v.Import {
		return true
	}
	reason := v.Reason
	if reason == "" {
		reason = "上架判定为不入库"
	}
	r.res.Skipped++
	r.recordJobFile(ctx, oc.cand.source, oc.path, store.ImportFileStatusSkipped, reason, 0)
	return false
}
