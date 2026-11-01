package store

// 本文件是「版本择优」的判定规则（设计 §5）：命中率到阈值之后，用质量分回答
// 「新版本是不是更好的版本」。
//
// # 判定规则表
//
// 三个维度各自有「显著更优」与「显著更差」两个阈值（比率类都是越低越好）：
//
//	维度     显著更优                        显著更差
//	噪声率   new < old 且 new <= old x 50%    new > old 且 new >= old x 150%
//	乱码率   new < old 且 new <= old x 20%    new - old > 1e-9（任何上升）
//	结构分   new >= old + 0.10                new <= old - 0.10
//
// 结论：
//   - 显著更优 >= 2 项、且显著更差 = 0 项 → **升级候选**（等人工确认后才替换）；
//   - 其余（更优不足 2 项，或任一项显著更差）→ **重复**（保留旧版本）。
//
// 「更优」与「更差」都要求方向真的成立（new < old / new > old）：否则旧值恰好是 0 时
// 「new <= old x 50%」会退化成「new <= 0」、「new >= old x 150%」会退化成恒真，
// 把「持平」判成「更差」。乱码率的下界用一个极小 epsilon 吸收浮点噪声
// （1e-9 远小于任何真实差异）。
//
// # 保守路径
//
// 旧版本或新版本没有质量分（EPUB / 合集拆出的子书 / 0020 之前入的存量书）时不猜：
// 一律判**重复**并在 Note 里说明缺的是哪一侧，由人工判断要不要保留新文件。
// 拿半份分数做「替换」判断，比漏判一次升级候选严重得多。

// 判定阈值（改这里就是改口径，测试逐条锁死）。
const (
	noiseBetterFactor    = 0.5  // 噪声率降到旧版本的 <=50% 算显著更优
	noiseWorseFactor     = 1.5  // 噪声率涨到旧版本的 >=150% 算显著更差
	garbageBetterFactor  = 0.2  // 乱码率降到旧版本的 <=20%（降幅 >=80%）
	garbageWorseEpsilon  = 1e-9 // 乱码率上升的判定下界（吸收浮点噪声）
	structureBetterDelta = 0.1  // 结构分升 >=0.10 算显著更优
	structureWorseDelta  = 0.1  // 结构分降 >=0.10 算显著更差
	minBetterDimensions  = 2    // 升级候选至少要几项显著更优
)

// 保守路径的说明文案（判定不成立的原因，进导入备注）。
const (
	VersionNoteOldUnknown = "旧版本无质量分"
	VersionNoteNewUnknown = "新版本无质量分"
)

// QualityDimension 是质量分对比里的一个维度：键 + 新旧取值。
//
// 只带数值不带文案：中文措辞属于导入链路的展示层（见 internal/importer/dedupe.go），
// store 只负责口径。
type QualityDimension struct {
	Key string  `json:"key"` // QualityDim* 之一
	Old float64 `json:"old"`
	New float64 `json:"new"`
}

// VersionPick 是一次「新旧版本质量对比」的结果。
type VersionPick struct {
	// Upgrade 为 true 表示升级候选：新版本至少两项显著更优且无一项显著更差。
	// 为 false 且 OldComputed 为真表示「旧版本更优或持平」。
	Upgrade     bool               `json:"upgrade"`
	OldComputed bool               `json:"oldComputed"`
	NewComputed bool               `json:"newComputed"`
	Old         BookQuality        `json:"old"`
	New         BookQuality        `json:"new"`
	Better      []QualityDimension `json:"better,omitempty"` // 显著更优的维度
	Worse       []QualityDimension `json:"worse,omitempty"`  // 显著更差的维度
	Note        string             `json:"note,omitempty"`   // 走保守路径的原因（空串 = 正常判定）
}

// compareVersionQuality 按上面的规则表对比新旧质量分。
func compareVersionQuality(oldQ, newQ BookQuality) VersionPick {
	p := VersionPick{Old: oldQ, New: newQ, OldComputed: oldQ.Computed(), NewComputed: newQ.Computed()}
	switch {
	case !p.OldComputed:
		p.Note = VersionNoteOldUnknown
		return p
	case !p.NewComputed:
		p.Note = VersionNoteNewUnknown
		return p
	}
	p.Better = betterDimensions(oldQ, newQ)
	p.Worse = worseDimensions(oldQ, newQ)
	p.Upgrade = len(p.Better) >= minBetterDimensions && len(p.Worse) == 0
	return p
}

// CompareBookQuality 是 compareVersionQuality 的导出入口。
//
// 给 api 的审核列表用：列表要展示「规则怎么判的」，必须与导入时的判定同一份口径，
// 展示端不重新实现阈值（否则同一对书在导入备注与审核页上会给出不同结论）。
func CompareBookQuality(oldQ, newQ BookQuality) VersionPick { return compareVersionQuality(oldQ, newQ) }

// betterDimensions 列出显著更优的维度（顺序固定：噪声 → 乱码 → 结构）。
func betterDimensions(oldQ, newQ BookQuality) []QualityDimension {
	out := make([]QualityDimension, 0, 3)
	if noiseImproved(oldQ.NoiseRatio, newQ.NoiseRatio) {
		out = append(out, QualityDimension{Key: QualityDimNoise, Old: oldQ.NoiseRatio, New: newQ.NoiseRatio})
	}
	if garbageImproved(oldQ.GarbageRatio, newQ.GarbageRatio) {
		out = append(out, QualityDimension{Key: QualityDimGarbage, Old: oldQ.GarbageRatio, New: newQ.GarbageRatio})
	}
	if structureImproved(oldQ.StructureScore, newQ.StructureScore) {
		out = append(out, QualityDimension{Key: QualityDimStructure, Old: oldQ.StructureScore, New: newQ.StructureScore})
	}
	return out
}

// worseDimensions 列出显著更差的维度（顺序同上）。
func worseDimensions(oldQ, newQ BookQuality) []QualityDimension {
	out := make([]QualityDimension, 0, 3)
	if noiseRegressed(oldQ.NoiseRatio, newQ.NoiseRatio) {
		out = append(out, QualityDimension{Key: QualityDimNoise, Old: oldQ.NoiseRatio, New: newQ.NoiseRatio})
	}
	if garbageRegressed(oldQ.GarbageRatio, newQ.GarbageRatio) {
		out = append(out, QualityDimension{Key: QualityDimGarbage, Old: oldQ.GarbageRatio, New: newQ.GarbageRatio})
	}
	if structureRegressed(oldQ.StructureScore, newQ.StructureScore) {
		out = append(out, QualityDimension{Key: QualityDimStructure, Old: oldQ.StructureScore, New: newQ.StructureScore})
	}
	return out
}

// noiseImproved 噪声率显著更优：确实下降且降到旧值的 <=50%。
func noiseImproved(oldV, newV float64) bool { return newV < oldV && newV <= oldV*noiseBetterFactor }

// noiseRegressed 噪声率显著更差：确实上升且涨到旧值的 >=150%。
//
// 旧值为 0 时任何上升都算「更差」（0 的 150% 仍是 0）：这是刻意选的保守方向——
// 回填出来的历史书噪声率恒为 0（正文入库时噪声行已被丢弃），新版本只要有噪声就不该
// 被推成升级候选。
func noiseRegressed(oldV, newV float64) bool { return newV > oldV && newV >= oldV*noiseWorseFactor }

// garbageImproved 乱码率显著更优：确实下降且降到旧值的 <=20%（降幅 >=80%）。
func garbageImproved(oldV, newV float64) bool { return newV < oldV && newV <= oldV*garbageBetterFactor }

// garbageRegressed 乱码率显著更差：任何有意义的上升（阈值见 garbageWorseEpsilon）。
func garbageRegressed(oldV, newV float64) bool { return newV-oldV > garbageWorseEpsilon }

// structureImproved 结构分显著更优：升幅 >= structureBetterDelta。
func structureImproved(oldV, newV float64) bool { return newV >= oldV+structureBetterDelta }

// structureRegressed 结构分显著更差：降幅 >= structureWorseDelta。
func structureRegressed(oldV, newV float64) bool { return newV <= oldV-structureWorseDelta }
