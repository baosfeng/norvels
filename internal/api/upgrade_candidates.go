package api

import "norvels/internal/importer"

// logUpgradeCandidates 把本次导入判出的「升级候选」逐条打进服务日志（设计 §5）。
//
// 这些候选**不会**被自动替换（设计 §7）：导入只把它们记进任务的文件备注
// （终态 skipped + 「升级候选：…」，用户能在 GET /api/import/jobs/{id}/files 里看到），
// 结构化字段由 importer.Result.UpgradeCandidates 交给调用方。
// 日志是运维侧的第一落点——替换 UI 还没做之前，用户靠它知道「有一本更好的版本在门外」。
func (s *Server) logUpgradeCandidates(res *importer.Result) {
	if res == nil {
		return
	}
	for _, c := range res.UpgradeCandidates {
		s.logf("api: 升级候选 %s（旧书 %d《%s》命中率 %.0f%%，更优 %d 项；未替换，需人工确认）",
			c.DisplayPath, c.OldBookID, c.OldBookTitle, c.HitRate*100, len(c.Better))
	}
}
