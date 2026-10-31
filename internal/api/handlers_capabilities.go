package api

import (
	"net/http"

	"norvels/internal/importer"
	"norvels/internal/parser"
	"norvels/internal/source"
)

// capabilitiesPath 是「服务端能力清单」端点：前端所有硬编码上限与策略名单的
// 唯一来源（前端只读它，不再各写一份数字）。
const capabilitiesPath = "/api/settings/capabilities"

// namedPriority 是过滤器清单元素（按 Priority 升序，即执行顺序）。
type namedPriority struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

// namedWeight 是元数据策略清单元素（按 Weight 降序，即投票顺位）。
type namedWeight struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

// ruleLimit 是章节规则的跨边界上限。
type ruleLimit struct {
	MaxCount        int `json:"maxCount"`
	MaxPatternRunes int `json:"maxPatternRunes"`
	MaxNameRunes    int `json:"maxNameRunes"`
}

// tagRuleLimit 是词表打标规则的跨边界上限。
type tagRuleLimit struct {
	MaxCount        int `json:"maxCount"`
	MaxTagRunes     int `json:"maxTagRunes"`
	MaxKeywords     int `json:"maxKeywords"`
	MaxKeywordRunes int `json:"maxKeywordRunes"`
}

// profileLimit 是来源规则包的跨边界上限。
type profileLimit struct {
	MaxCount           int `json:"maxCount"`
	MaxRulesPerProfile int `json:"maxRulesPerProfile"`
	MaxPatternRunes    int `json:"maxPatternRunes"`
	MaxNameRunes       int `json:"maxNameRunes"`
}

// uploadCap 是本次进程生效的上传上限（随 Config 注入，测试会注入小值）。
type uploadCap struct {
	MaxFileBytes  int64 `json:"maxFileBytes"`
	MaxTotalBytes int64 `json:"maxTotalBytes"`
	MaxFiles      int   `json:"maxFiles"`
}

// importCap 是导入线程的规模参数与解析默认值。
type importCap struct {
	MaxWorkers                int `json:"maxWorkers"`
	DefaultMaxChapterChars    int `json:"defaultMaxChapterChars"`
	DefaultFallbackChunkChars int `json:"defaultFallbackChunkChars"`
}

// dryRunCap 是 dry-run 预检的规模上限：与 importer 的导出常量同源，
// 前端不再各写一份「200 / 20 / 50」。
type dryRunCap struct {
	MaxFiles int `json:"maxFiles"`
	Samples  int `json:"samples"`
	Errors   int `json:"errors"`
}

// capabilitiesLimits 汇总各域上限；只读下发，不接受写入（护栏不接受配置）。
type capabilitiesLimits struct {
	ChapterRules   ruleLimit    `json:"chapterRules"`
	TagRules       tagRuleLimit `json:"tagRules"`
	SourceProfiles profileLimit `json:"sourceProfiles"`
	Upload         uploadCap    `json:"upload"`
	Import         importCap    `json:"import"`
	DryRun         dryRunCap    `json:"dryRun"`
}

// capabilitiesResponse 是 GET /api/settings/capabilities 的响应。
type capabilitiesResponse struct {
	Splitters  []string           `json:"splitters"`
	Codecs     []string           `json:"codecs"`
	Filters    []namedPriority    `json:"filters"`
	Metas      []namedWeight      `json:"metas"`
	Extensions []string           `json:"extensions"`
	Limits     capabilitiesLimits `json:"limits"`
}

// handleCapabilities 处理 GET /api/settings/capabilities。
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.capabilities())
}

// capabilities 从策略注册表与现有常量拼装能力清单，不引入第二份真值：
// 改名/加策略/调上限只改一处，前端跟着下发值走。
func (s *Server) capabilities() capabilitiesResponse {
	defaults := parser.DefaultOptions()
	upload := s.cfg.uploadLimits()
	return capabilitiesResponse{
		Splitters:  splitterNames(parser.Splitters()),
		Codecs:     codecNames(parser.Codecs()),
		Filters:    filterCaps(parser.Filters()),
		Metas:      metaCaps(parser.Metas()),
		Extensions: []string{importer.FormatTXT, importer.FormatEPUB, importer.FormatZIP},
		Limits: capabilitiesLimits{
			ChapterRules: ruleLimit{
				MaxCount:        maxChapterRules,
				MaxPatternRunes: maxChapterRuleRunes,
				MaxNameRunes:    maxChapterRuleNameRunes,
			},
			TagRules: tagRuleLimit{
				MaxCount:        maxTagRules,
				MaxTagRunes:     maxTagRuleRunes,
				MaxKeywords:     maxTagRuleKeywords,
				MaxKeywordRunes: maxTagKeywordRunes,
			},
			SourceProfiles: profileLimit{
				MaxCount:           source.MaxProfiles,
				MaxRulesPerProfile: source.MaxRulesPerProfile,
				MaxPatternRunes:    source.MaxPatternRunes,
				MaxNameRunes:       source.MaxNameRunes,
			},
			Upload: uploadCap{
				MaxFileBytes:  upload.fileBytes,
				MaxTotalBytes: upload.totalBytes,
				MaxFiles:      upload.files,
			},
			Import: importCap{
				MaxWorkers:                maxImportWorkers,
				DefaultMaxChapterChars:    defaults.MaxChapterChars,
				DefaultFallbackChunkChars: defaults.FallbackChunkChars,
			},
			DryRun: dryRunCap{
				MaxFiles: importer.DryRunMaxFiles,
				Samples:  importer.DryRunMaxSamples,
				Errors:   importer.DryRunMaxErrors,
			},
		},
	}
}

// splitterNames 提取切章策略名（按注册顺序）。
func splitterNames(list []parser.Splitter) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name())
	}
	return out
}

// codecNames 提取编码策略名（按注册顺序，即编码候选的优先顺序）。
func codecNames(list []parser.Codec) []string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.Name())
	}
	return out
}

// filterCaps 提取过滤器清单（parser.Filters 已按 Priority 升序）。
func filterCaps(list []parser.Filter) []namedPriority {
	out := make([]namedPriority, 0, len(list))
	for _, f := range list {
		out = append(out, namedPriority{Name: f.Name(), Priority: f.Priority()})
	}
	return out
}

// metaCaps 提取元数据策略清单（parser.Metas 已按 Weight 降序）。
func metaCaps(list []parser.MetaExtractor) []namedWeight {
	out := make([]namedWeight, 0, len(list))
	for _, m := range list {
		out = append(out, namedWeight{Name: m.Name(), Weight: m.Weight()})
	}
	return out
}
