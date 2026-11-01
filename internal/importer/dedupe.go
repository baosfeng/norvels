package importer

import (
	"context"
	"fmt"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件是导入链路上的内容去重判定（设计 §4）：写库前判一次，按命中率决定
// 「按重复跳过」还是「带备注导入」。
//
// 分档与阈值口径都在 store 侧（DedupeReport.Level + settings 的 dedupe.threshold），
// 这里只负责「拿结论 → 翻译成计数与文件备注」，不重新定义百分比。

// judgeDuplicate 在 saveOne 之前判重，返回 (是否按重复跳过, 文件备注)。
//
//   - 命中率 ≥ 阈值：按 Skipped 处理，备注写「疑似重复：库内已有 X% 相同内容（《书名》可对照）」；
//   - 40% ≤ 命中率 < 阈值：正常导入，备注写「疑似相关：X%」；
//   - < 40%：正常导入，无备注。
//
// Force 与 Update 一律跳过判定：这两条路径都是用户**显式声明「这是同一本书」**
// （覆盖 / 追更），此时判重只会误伤——追更版本天然包含旧版本的全部章节，命中率必然
// 接近 100%，判成「疑似重复」就等于追更永远不生效。Force 的语义与 store.SaveBook 一致。
// 判定失败也不拦导入——判重是提示，不是准入条件；原因进 lastErr 让任务记录带上。
func (r *runState) judgeDuplicate(ctx context.Context, source, display string,
	book *model.ParsedBook, opt Options) (skipped bool, note string) {
	if opt.Force || opt.Update || len(book.Chapters) == 0 {
		return false, ""
	}
	rep, err := r.st.CheckContentDuplicate(ctx, book.Chapters)
	if err != nil {
		r.lastErr = err.Error()
		return false, ""
	}
	switch rep.Level(opt.dedupeThreshold()) {
	case store.DedupeLevelDuplicate:
		r.res.Skipped++
		r.recordJobFile(ctx, source, display, store.ImportFileStatusSkipped, duplicateReason(rep), 0)
		return true, ""
	case store.DedupeLevelRelated:
		return false, relatedNote(rep)
	}
	return false, ""
}

// dedupeThreshold 取本次导入的阈值：0 或越界一律回落缺省（阈值错不该让导入停下）。
func (o Options) dedupeThreshold() float64 {
	if o.DedupeThreshold > 0 && o.DedupeThreshold <= 1 {
		return o.DedupeThreshold
	}
	return store.DefaultDedupeThreshold
}

// duplicateReason 生成「按重复跳过」的文件备注（文案口径见设计 §4）。
//
// 带上书名是刻意的：命中率能说明「像」，书名让人不用再查库就知道该跟哪本对照。
// 书已被删（指纹成孤儿）时写「书名未知」，不静默编一个名字。
func duplicateReason(rep store.DedupeReport) string {
	title := rep.DuplicateBookTitle
	if title == "" {
		title = "书名未知"
	}
	return fmt.Sprintf("疑似重复：库内已有 %s 相同内容（《%s》可对照）", percentText(rep.HitRate), title)
}

// relatedNote 生成「疑似相关」的备注：只写百分比，终态仍是 imported。
func relatedNote(rep store.DedupeReport) string {
	return "疑似相关：" + percentText(rep.HitRate)
}

// percentText 把命中率格式化成整数百分比（99.6% 显示 100%，不做小数）。
func percentText(rate float64) string {
	return fmt.Sprintf("%.0f%%", rate*100)
}

// joinReason 拼接两条文件备注，空串不产生多余分隔符。
func joinReason(first, second string) string {
	switch {
	case second == "":
		return first
	case first == "":
		return second
	}
	return first + "；" + second
}
