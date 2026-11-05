package importer

import (
	"testing"

	"norvels/internal/store"
)

// TestQualityDimTextStructure 结构分是 0~1 的量纲分，按两位小数渲染；
// 比率类维度走百分比（见 ratioText）。两者不能混用同一格式。
func TestQualityDimTextStructure(t *testing.T) {
	got := qualityDimText(store.QualityDimension{Key: store.QualityDimStructure, Old: 0.42, New: 0.91})
	if want := "结构分 0.42→0.91"; got != want {
		t.Fatalf("qualityDimText(结构分) = %q，want %q", got, want)
	}
	ratio := qualityDimText(store.QualityDimension{Key: store.QualityDimNoise, Old: 0.32, New: 0.04})
	if want := "噪声率 32.0%→4.0%"; ratio != want {
		t.Fatalf("qualityDimText(噪声率) = %q，want %q", ratio, want)
	}
}
