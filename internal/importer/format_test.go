package importer

import (
	"strings"
	"testing"
)

// TestSupportedFormats 错误提示里列出的支持格式与 FormatOf 认得的扩展名一致。
func TestSupportedFormats(t *testing.T) {
	got := SupportedFormats()
	want := []string{"TXT", "EPUB", "ZIP"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("SupportedFormats = %v，want %v", got, want)
	}
	for _, name := range []string{"a.txt", "b.epub", "c.zip"} {
		if !SupportedExt(name) {
			t.Errorf("SupportedExt(%q) = false，但它出现在 SupportedFormats 里", name)
		}
	}
}
