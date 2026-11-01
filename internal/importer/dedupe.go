package importer

import (
	"context"
	"fmt"
	"strings"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 本文件是导入链路上的内容去重判定（设计 §4）与版本择优（设计 §5）：写库前判一次，
// 按命中率决定「按重复跳过」还是「带备注导入」，同一本书再比一次质量分决定
// 「是升级候选」还是「库内版本更优/持平」。
//
// 分档与阈值口径都在 store 侧（DedupeReport.Level + settings 的 dedupe.threshold，
// 质量对比规则在 store 的 versionpick.go），这里只负责「拿结论 → 翻译成计数与文件备注」，
// 不重新定义百分比，也不重新定义「显著更优」。

// dupDecision 是写库前的判重结论（设计 §4 / §5 / §7）。
//
// 三种结局互斥：skip（纯重复，不入库）、upgrade（升级候选，入库但隐藏 + 落审核记录）、
// 空（正常导入，note 可能是「疑似相关」备注）。
type dupDecision struct {
	skip    bool   // true = 按重复跳过，不入库
	note    string // 文件备注（可空）
	upgrade *UpgradeCandidate
}

// judgeDuplicate 在 saveOne 之前判重并做版本择优。
//
//   - 命中率 >= 阈值且**新版本显著更优**：照常入库（入库即标记 superseded，书架不会
//     多出一本重复的书），写库后再落一条 pending 审核记录 —— 替换 / 回滚都只改两个
//     整数字段，不需要重新导入（设计 §7）；
//   - 命中率 >= 阈值、其余情形：按 Skipped 处理，备注写「疑似重复：…」并说明
//     库内版本更优 / 持平 / 无质量分（保守路径）；
//   - 40% <= 命中率 < 阈值：正常导入，备注写「疑似相关：X%」；
//   - < 40%：正常导入，无备注。
//
// Force 与 Update 一律跳过判定：这两条路径都是用户**显式声明「这是同一本书」**
// （覆盖 / 追更），此时判重只会误伤——追更版本天然包含旧版本的全部章节，命中率必然
// 接近 100%，判成「疑似重复」就等于追更永远不生效。Force 的语义与 store.SaveBook 一致。
// 判定失败也不拦导入——判重是提示，不是准入条件；原因进 lastErr 让任务记录带上。
func (r *runState) judgeDuplicate(ctx context.Context, source, display string,
	book *model.ParsedBook, opt Options) dupDecision {
	if opt.Force || opt.Update || len(book.Chapters) == 0 {
		return dupDecision{}
	}
	rep, err := r.st.CheckBookDuplicate(ctx, book, opt.dedupeThreshold())
	if err != nil {
		r.lastErr = err.Error()
		return dupDecision{}
	}
	switch rep.Level(opt.dedupeThreshold()) {
	case store.DedupeLevelDuplicate:
		if cand, ok := upgradeCandidate(source, display, rep); ok {
			return dupDecision{upgrade: &cand}
		}
		return dupDecision{skip: true, note: r.recordDuplicate(ctx, source, display, rep)}
	case store.DedupeLevelRelated:
		return dupDecision{note: relatedNote(rep)}
	}
	return dupDecision{}
}

// recordDuplicate 记录一次「纯重复」判定：计数 + 文件备注（终态 skipped）。
//
// 返回空串：跳过时不再需要额外的文件备注（说明已经写进文件终态）。
func (r *runState) recordDuplicate(ctx context.Context, source, display string, rep store.DedupeReport) string {
	r.res.Skipped++
	r.recordJobFile(ctx, source, display, store.ImportFileStatusSkipped, duplicateReason(rep), 0)
	return ""
}

// recordUpgrade 给一次升级候选落审核记录，返回文件备注（终态 imported）。
//
// 记录落不成时**不拦导入**：新版本已经入库且可见，用户至少还能自己比对；
// 原因进 lastErr 让任务记录带上，不做静默降级。
func (r *runState) recordUpgrade(ctx context.Context, cand UpgradeCandidate, newBookID int64) string {
	cand.NewBookID = newBookID
	id, err := r.st.CreateImportReview(ctx, store.NewReview{
		JobID:       r.jobID(),
		NewBookID:   newBookID,
		OldBookID:   cand.OldBookID,
		HitRatio:    cand.HitRate,
		RuleReasons: dimensionReasons(cand.Better),
	})
	if err != nil {
		r.lastErr = err.Error()
		return upgradeNote(cand, 0) + "；审核记录创建失败，新版本已按普通书入库"
	}
	cand.ReviewID = id
	r.res.UpgradeCandidates = append(r.res.UpgradeCandidates, cand)
	return upgradeNote(cand, id)
}

// jobID 返回当前任务 id（dry-run 或未建任务时为 0）。
func (r *runState) jobID() int64 {
	if r.job == nil {
		return 0
	}
	return r.job.ID
}

// dedupeThreshold 取本次导入的阈值：0 或越界一律回落缺省（阈值错不该让导入停下）。
func (o Options) dedupeThreshold() float64 {
	if o.DedupeThreshold > 0 && o.DedupeThreshold <= 1 {
		return o.DedupeThreshold
	}
	return store.DefaultDedupeThreshold
}

// duplicateReason 生成「按重复跳过」的文件备注（文案口径见设计 §4 / §5）。
//
// 带上书名是刻意的：命中率能说明「像」，书名让人不用再查库就知道该跟哪本对照。
// 书已被删（指纹成孤儿）时写「书名未知」，不静默编一个名字。质量分那一侧只写结论
// （更优 / 持平 / 无质量分），具体数值留给升级候选——重复时用户不需要逐项数字。
func duplicateReason(rep store.DedupeReport) string {
	return fmt.Sprintf("疑似重复：库内已有 %s 相同内容（《%s》%s）",
		percentText(rep.HitRate), bookTitleOrUnknown(rep), versionCompareText(rep))
}

// versionCompareText 描述库内版本与新版本的质量关系（设计 §5 的保守路径也在这里）。
func versionCompareText(rep store.DedupeReport) string {
	if rep.Version == nil {
		return "未做质量对比"
	}
	switch {
	case rep.Version.Note != "":
		return rep.Version.Note + "，按重复保留库内版本"
	case len(rep.Version.Worse) > 0:
		return "质量更优，保留库内版本"
	}
	return "质量持平，保留库内版本"
}

// upgradeNote 生成「升级候选」的文件备注：逐项列出显著更优的维度 + 处置提示。
//
// 带上审核记录 id 是刻意的：用户看到这条备注就能直接调
// GET /api/import/reviews 定位到对应记录；reviewID 为 0 表示记录没落成（新版本可见）。
func upgradeNote(cand UpgradeCandidate, reviewID int64) string {
	tail := "需人工确认替换，本轮不自动替换"
	if reviewID > 0 {
		tail = fmt.Sprintf("已入库并隐藏，待审核 #%d，确认前不出现在书架", reviewID)
	}
	return fmt.Sprintf("升级候选：%s（旧版本《%s》质量更差，%s）",
		dimensionsText(cand.Better), titleOrUnknown(cand.OldBookTitle), tail)
}

// titleOrUnknown 取书名，缺失时写「书名未知」。
func titleOrUnknown(title string) string {
	if title == "" {
		return "书名未知"
	}
	return title
}

// dimensionReasons 把「显著更优的维度」翻成落库用的规则对比文案。
//
// 与文件备注同一份文案（dimensionsText）：审核页展示的就是导入时算出来的那句
// 「噪声率 3.2%→0.4%」，两处不会漂。
func dimensionReasons(dims []store.QualityDimension) []string {
	out := make([]string, 0, len(dims))
	for _, d := range dims {
		out = append(out, qualityDimText(d))
	}
	return out
}

// bookTitleOrUnknown 取命中书书名，书已删时写「书名未知」。
func bookTitleOrUnknown(rep store.DedupeReport) string {
	if rep.DuplicateBookTitle == "" {
		return "书名未知"
	}
	return rep.DuplicateBookTitle
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

// 质量维度的中文名（键在 store 侧，文案在这里）。
var qualityDimLabels = map[string]string{
	store.QualityDimNoise:     "噪声率",
	store.QualityDimGarbage:   "乱码率",
	store.QualityDimStructure: "结构分",
}

// dimensionsText 把一组维度拼成「噪声率 3.2%→0.4%、结构分 0.42→0.91」。
func dimensionsText(dims []store.QualityDimension) string {
	parts := make([]string, 0, len(dims))
	for _, d := range dims {
		parts = append(parts, qualityDimText(d))
	}
	return strings.Join(parts, "、")
}

// qualityDimText 渲染单个维度：比率类按百分比、结构分按 0~1 两位小数。
func qualityDimText(d store.QualityDimension) string {
	label := qualityDimLabels[d.Key]
	if d.Key == store.QualityDimStructure {
		return fmt.Sprintf("%s %.2f→%.2f", label, d.Old, d.New)
	}
	return label + " " + ratioText(d.Old) + "→" + ratioText(d.New)
}

// ratioText 渲染一个 0~1 的比率：常规按一位小数百分比，极小的值多给两位。
//
// 为什么要多给：乱码率常见量级是 0.01%（几万字里几个 U+FFFD），一位小数会印成
// 「0.0%→0.0%」——用户看不出改善，而这正是升级候选的理由。
func ratioText(v float64) string {
	pct := v * 100
	if pct > 0 && pct < 0.1 {
		return fmt.Sprintf("%.3f%%", pct)
	}
	return fmt.Sprintf("%.1f%%", pct)
}
