package api

import (
	"net/http"
	"reflect"
	"testing"

	"norvels/internal/importer"
	"norvels/internal/parser"
	"norvels/internal/source"
)

// TestCapabilities 能力清单：策略名来自注册表、上限来自常量与配置，字段全部非空。
func TestCapabilities(t *testing.T) {
	e := newEnv(t)
	rr := e.get(capabilitiesPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d", rr.Code)
	}
	got := decodeBody[capabilitiesResponse](t, rr)
	requireCapabilityLists(t, got)
	requireCapabilityLimits(t, got.Limits)
	requireRuntimeCapabilities(t, got)
}

// requireRuntimeCapabilities 断言「运行时能力」三组：推荐 tab、埋点契约、AI 可用性。
func requireRuntimeCapabilities(t *testing.T, got capabilitiesResponse) {
	t.Helper()
	requireRecommendCap(t, got.Recommend)
	requireEventsCap(t, got.Events)
	requireAICap(t, got.AI)
	// 必须是列表页真实生效的 size 上限（maxSize），不是分页页码的夹取范围（maxPage）：
	// 下发一个 1000000 会让前端「按服务端上限取一页」直接把整库拉下来。
	if got.Limits.Pagination.MaxSize != maxSize {
		t.Fatalf("分页上限 = %d，期望 %d", got.Limits.Pagination.MaxSize, maxSize)
	}
}

// requireRecommendCap 断言推荐 tab 名册与 limit 上下限。
func requireRecommendCap(t *testing.T, cap recommendCap) {
	t.Helper()
	if cap.Enabled {
		t.Fatal("未打开推荐库时 recommend.enabled 应为 false（前端据此整块不渲染）")
	}
	if len(cap.Tabs) != 3 || cap.MaxLimit <= 0 || cap.DefaultLimit <= 0 {
		t.Fatalf("推荐能力不完整: %+v", cap)
	}
}

// requireEventsCap 断言埋点契约完整，且 kind 清单覆盖 dislike。
func requireEventsCap(t *testing.T, cap eventsCap) {
	t.Helper()
	if cap.MaxBatch == 0 || cap.MaxFieldLength == 0 || len(cap.Kinds) == 0 {
		t.Fatalf("埋点契约不完整: %+v", cap)
	}
	// 以前报错文案漏了 dislike，用户照提示改仍然 400。
	if !hasString(cap.Kinds, "dislike") {
		t.Fatalf("events.kinds 缺 dislike: %v", cap.Kinds)
	}
}

// requireAICap 断言没有 key 时 AI 两项都报 false。
func requireAICap(t *testing.T, cap aiCap) {
	t.Helper()
	if cap.ReviewAvailable || cap.CurationAvailable {
		t.Fatalf("没有 key 时 AI 两项都应为 false: %+v", cap)
	}
}

// requireCapabilityLists 断言策略清单非空、含代表性内置策略、排序口径正确。
func requireCapabilityLists(t *testing.T, got capabilitiesResponse) {
	t.Helper()
	if len(got.Splitters) == 0 || len(got.Codecs) == 0 || len(got.Filters) == 0 || len(got.Metas) == 0 {
		t.Fatalf("能力清单有空项: %+v", got)
	}
	if !hasString(got.Splitters, "di-unit") {
		t.Errorf("splitters 缺内置策略 di-unit: %v", got.Splitters)
	}
	if !hasString(got.Codecs, "utf-8") {
		t.Errorf("codecs 缺 utf-8: %v", got.Codecs)
	}
	requireSorted(t, got.Filters, got.Metas)
	if want := []string{"txt", "epub", "zip"}; !reflect.DeepEqual(got.Extensions, want) {
		t.Errorf("extensions = %v，期望 %v", got.Extensions, want)
	}
}

// requireSorted 断言过滤器按 Priority 升序（执行顺序）、元数据按 Weight 降序（投票顺位）。
func requireSorted(t *testing.T, filters []namedPriority, metas []namedWeight) {
	t.Helper()
	for i := 1; i < len(filters); i++ {
		if filters[i-1].Priority > filters[i].Priority {
			t.Errorf("filters 未按 priority 升序: %+v", filters)
		}
	}
	for i := 1; i < len(metas); i++ {
		if metas[i-1].Weight < metas[i].Weight {
			t.Errorf("metas 未按 weight 降序: %+v", metas)
		}
	}
}

// TestCapabilitiesReflectsConfig 上传上限随 Config 注入（不是另一份写死的常量）。
func TestCapabilitiesReflectsConfig(t *testing.T) {
	e := newEnvWith(t, Config{
		DisableAccessLog:    true,
		MaxUploadFileBytes:  1234,
		MaxUploadTotalBytes: 5678,
		MaxUploadFiles:      3,
	}, testFS())
	got := decodeBody[capabilitiesResponse](t, e.get(capabilitiesPath))
	if got.Limits.Upload.MaxFileBytes != 1234 || got.Limits.Upload.MaxTotalBytes != 5678 ||
		got.Limits.Upload.MaxFiles != 3 {
		t.Errorf("upload 上限未跟随 Config: %+v", got.Limits.Upload)
	}
}

// requireCapabilityLimits 断上限与常量/默认值一致（防止下发值悄悄漂移）。
func requireCapabilityLimits(t *testing.T, l capabilitiesLimits) {
	t.Helper()
	defaults := parser.DefaultOptions()
	checks := []struct {
		name      string
		got, want int
	}{
		{"chapterRules.maxCount", l.ChapterRules.MaxCount, maxChapterRules},
		{"chapterRules.maxPatternRunes", l.ChapterRules.MaxPatternRunes, maxChapterRuleRunes},
		{"tagRules.maxCount", l.TagRules.MaxCount, maxTagRules},
		{"tagRules.maxKeywords", l.TagRules.MaxKeywords, maxTagRuleKeywords},
		{"sourceProfiles.maxCount", l.SourceProfiles.MaxCount, source.MaxProfiles},
		{"sourceProfiles.maxRulesPerProfile", l.SourceProfiles.MaxRulesPerProfile, source.MaxRulesPerProfile},
		{"sourceProfiles.maxPatternRunes", l.SourceProfiles.MaxPatternRunes, source.MaxPatternRunes},
		{"import.maxWorkers", l.Import.MaxWorkers, maxImportWorkers},
		{"import.defaultMaxChapterChars", l.Import.DefaultMaxChapterChars, defaults.MaxChapterChars},
		{"import.defaultFallbackChunkChars", l.Import.DefaultFallbackChunkChars, defaults.FallbackChunkChars},
		{"dryRun.maxFiles", l.DryRun.MaxFiles, importer.DryRunMaxFiles},
		{"dryRun.samples", l.DryRun.Samples, importer.DryRunMaxSamples},
		{"dryRun.errors", l.DryRun.Errors, importer.DryRunMaxErrors},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d，期望 %d", c.name, c.got, c.want)
		}
	}
	if l.Upload.MaxFileBytes != defaultMaxUploadFileBytes || l.Upload.MaxTotalBytes != defaultMaxUploadTotalBytes ||
		l.Upload.MaxFiles != defaultMaxUploadFiles {
		t.Errorf("upload 上限 = %+v，期望默认值 %d/%d/%d", l.Upload,
			defaultMaxUploadFileBytes, defaultMaxUploadTotalBytes, defaultMaxUploadFiles)
	}
}

// hasString 判断字符串切片是否包含目标值。
func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
