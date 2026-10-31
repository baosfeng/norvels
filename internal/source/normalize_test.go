package source

import (
	"reflect"
	"strings"
	"testing"
)

// TestNormalizeDefaults 归一化补齐缺省值、小写扩展名、去点与去重。
func TestNormalizeDefaults(t *testing.T) {
	got := Normalize([]Profile{{
		ID: " demo ", Name: " 示例 ",
		Match:  Match{Ext: []string{".TXT", "TXT", " .Epub ", ""}},
		Format: []string{".txt", "txt"},
		Decode: Decode{ForceEncoding: " GB18030 "},
	}})[0]
	if got.ID != "demo" || got.Name != "示例" {
		t.Errorf("id/name = %q/%q，期望 demo/示例", got.ID, got.Name)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %d，期望 %d", got.SchemaVersion, SchemaVersion)
	}
	if got.Kind != KindLocal {
		t.Errorf("kind = %q，期望 %q", got.Kind, KindLocal)
	}
	if want := []string{"txt", "epub"}; !reflect.DeepEqual(got.Match.Ext, want) {
		t.Errorf("match.ext = %v，期望 %v", got.Match.Ext, want)
	}
	if want := []string{"txt"}; !reflect.DeepEqual(got.Format, want) {
		t.Errorf("format = %v，期望 %v", got.Format, want)
	}
	if got.Decode.ForceEncoding != "gb18030" {
		t.Errorf("forceEncoding = %q，期望 gb18030", got.Decode.ForceEncoding)
	}
}

// TestNormalizeRules 归一化过滤/切章/元数据规则名（留空补名）与标签关键词（去空去重）。
func TestNormalizeRules(t *testing.T) {
	got := Normalize([]Profile{{
		ID: "demo", Name: "示例",
		Filters: Filters{Disable: []string{" width "}, Extra: []FilterRule{{Pattern: " ^x$ "}}},
		Split:   Split{ForceSplitter: " Di-Unit ", Rules: []Rule{{Pattern: "^x$"}}},
		Meta:    []MetaRule{{Pattern: "^y$", Title: 1}},
		Tags:    []TagRule{{Tag: "奇幻", Keywords: []string{" 魔法 ", "", "魔法"}}},
	}})[0]
	if got.Filters.Disable[0] != "width" {
		t.Errorf("disable = %q，期望 width", got.Filters.Disable[0])
	}
	if got.Filters.Extra[0].Name != "extra-1" || got.Filters.Extra[0].Pattern != "^x$" {
		t.Errorf("extra = %+v，期望名字补 extra-1 且 pattern 去空白", got.Filters.Extra[0])
	}
	if got.Split.ForceSplitter != "di-unit" {
		t.Errorf("forceSplitter = %q，期望 di-unit", got.Split.ForceSplitter)
	}
	if got.Split.Rules[0].Name != "规则1" {
		t.Errorf("切章规则名 = %q，期望 规则1", got.Split.Rules[0].Name)
	}
	if got.Meta[0].Name != "meta-1" {
		t.Errorf("元数据规则名 = %q，期望 meta-1", got.Meta[0].Name)
	}
	if want := []string{"魔法"}; !reflect.DeepEqual(got.Tags[0].Keywords, want) {
		t.Errorf("keywords = %v，期望 %v", got.Tags[0].Keywords, want)
	}
}

// TestNormalizeTruncatesLongNames 规则名只用于展示：超长截断而不是拒绝保存。
func TestNormalizeTruncatesLongNames(t *testing.T) {
	long := strings.Repeat("长", MaxNameRunes+10)
	got := Normalize([]Profile{{
		ID: "demo", Name: "示例",
		Filters: Filters{Extra: []FilterRule{{Name: long, Pattern: "x"}}},
	}})
	if n := len([]rune(got[0].Filters.Extra[0].Name)); n != MaxNameRunes {
		t.Errorf("规则名长度 = %d，期望 %d", n, MaxNameRunes)
	}
	// 规则包本名是主标题：超长不截断（截断会让用户以为改名了），交给 Validate 拒绝。
	p := got[0]
	p.Name = long
	if err := p.Validate(); err == nil || err.Code != CodeParam {
		t.Errorf("超长规则包名应被拒绝: %v", err)
	}
}

// TestNormalizeOnline 在线段：URL/分组去空白，规则块丢弃空键、去值空白。
func TestNormalizeOnline(t *testing.T) {
	in := []Profile{{
		ID: "demo", Name: "示例", Kind: KindOnline,
		Online: &Online{
			BookSourceURL:     " https://example.invalid/book ",
			Group:             " 示例组 ",
			SearchURL:         " https://example.invalid/search?q={{key}} ",
			RuleSearch:        map[string]string{" name ": " .title ", "": "drop"},
			RuleExplore:       map[string]string{" bookList ": " .item "},
			Header:            map[string]string{" User-Agent ": " 示例 "},
			UnsupportedFields: []string{" header ", "header", ""},
		},
	}}
	got := Normalize(in)[0]
	if got.Online.BookSourceURL != "https://example.invalid/book" {
		t.Errorf("url = %q", got.Online.BookSourceURL)
	}
	if got.Online.Group != "示例组" {
		t.Errorf("group = %q", got.Online.Group)
	}
	if want := map[string]string{"name": ".title"}; !reflect.DeepEqual(got.Online.RuleSearch, want) {
		t.Errorf("ruleSearch = %v，期望 %v", got.Online.RuleSearch, want)
	}
	if got.Online.SearchURL != "https://example.invalid/search?q={{key}}" {
		t.Errorf("searchUrl = %q", got.Online.SearchURL)
	}
	if want := map[string]string{"bookList": ".item"}; !reflect.DeepEqual(got.Online.RuleExplore, want) {
		t.Errorf("ruleExplore = %v，期望 %v", got.Online.RuleExplore, want)
	}
	if want := map[string]string{"User-Agent": "示例"}; !reflect.DeepEqual(got.Online.Header, want) {
		t.Errorf("header = %v，期望 %v", got.Online.Header, want)
	}
	if want := []string{"header"}; !reflect.DeepEqual(got.Online.UnsupportedFields, want) {
		t.Errorf("unsupportedFields = %v，期望 %v", got.Online.UnsupportedFields, want)
	}
}

// TestNormalizeEmpty 空输入返回空切片（非 nil），API 序列化成 []。
func TestNormalizeEmpty(t *testing.T) {
	if got := Normalize(nil); got == nil || len(got) != 0 {
		t.Errorf("Normalize(nil) = %#v，期望空切片", got)
	}
}
