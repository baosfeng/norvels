package importer

import "norvels/internal/store"

// 本文件是「升级候选」的结构化形状（设计 §5 / §7）。
//
// 判定在 store（versionpick.go），文案在 dedupe.go；这里只负责把「比出来了什么」
// 变成可被 API 直接消费的数据：旧书 id / 旧书名 / 命中率 / 新旧质量分 / 逐项差异。
//
// 刻意**不**包含「替换动作」：替换 = 旧书标 superseded + 新书成为当前版本，
// 必须人工确认（设计 §7），本轮只做判定与暴露字段。

// UpgradeCandidate 是一条「新版本显著更优」的记录。
type UpgradeCandidate struct {
	Source       string                   `json:"source"`       // 可重放的源（绝对路径 / 包:包内路径）
	DisplayPath  string                   `json:"displayPath"`  // 给用户看的路径
	OldBookID    int64                    `json:"oldBookId"`    // 库内版本的书 id（对照与未来替换用）
	OldBookTitle string                   `json:"oldBookTitle"` // 库内版本的书名（书已删时为空串）
	HitRate      float64                  `json:"hitRate"`      // 章级命中率（判定依据）
	Old          store.BookQuality        `json:"old"`
	New          store.BookQuality        `json:"new"`
	Better       []store.QualityDimension `json:"better"`          // 显著更优的维度
	Worse        []store.QualityDimension `json:"worse,omitempty"` // 恒为空（有更差就不是升级候选）
}

// upgradeCandidate 把一次判重报告翻译成升级候选；不是升级候选时返回 ok=false。
func upgradeCandidate(source, display string, rep store.DedupeReport) (UpgradeCandidate, bool) {
	if rep.Version == nil || !rep.Version.Upgrade {
		return UpgradeCandidate{}, false
	}
	return UpgradeCandidate{
		Source:       source,
		DisplayPath:  display,
		OldBookID:    rep.DuplicateBookID,
		OldBookTitle: rep.DuplicateBookTitle,
		HitRate:      rep.HitRate,
		Old:          rep.Version.Old,
		New:          rep.Version.New,
		Better:       rep.Version.Better,
		Worse:        rep.Version.Worse,
	}, true
}
