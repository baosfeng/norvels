package source

import (
	"fmt"
	"strings"
	"testing"
)

// validProfile 是一份各层都填了东西的合法规则包，供「只改一处」的表驱动用例复用。
func validProfile() Profile {
	return Profile{
		ID: "sample-dir", Name: "示例目录", SchemaVersion: 1, Enabled: true, Priority: 10,
		Kind:  KindLocal,
		Match: Match{Ext: []string{"txt"}, PathGlob: "/books/示例/**"},
		Filters: Filters{
			Disable: []string{"width"},
			Extra:   []FilterRule{{Name: "广告行", Priority: 30, Pattern: "^广告：.*$"}},
		},
		Meta:   []MetaRule{{Name: "头部作者", Weight: 60, Pattern: "^作者：(.+)$", Author: 1}},
		Split:  Split{ForceSplitter: "di-unit", Rules: []Rule{{Name: "分节符", Pattern: "^=== 第\\d+ 节 ===$"}}},
		Tags:   []TagRule{{Tag: "奇幻", Keywords: []string{"魔法"}}},
		Limits: Limits{MaxChapterChars: 20000, MinChapters: 1},
	}
}

// TestValidateAccepts 合法规则包通过；schemaVersion/kind 缺省也能通过（归一化前的宽容）。
func TestValidateAccepts(t *testing.T) {
	p := validProfile()
	if err := p.Validate(); err != nil {
		t.Fatalf("合法规则包被拒: %v", err)
	}
	p.SchemaVersion, p.Kind = 0, ""
	if err := p.Validate(); err != nil {
		t.Fatalf("缺省 schemaVersion/kind 应通过: %v", err)
	}
	online := validProfile()
	online.Kind = KindOnline
	online.Online = &Online{
		BookSourceURL: "https://example.invalid/book",
		Group:         "示例组",
		RuleSearch:    map[string]string{"name": ".title"},
	}
	if err := online.Validate(); err != nil {
		t.Fatalf("合法在线规则包被拒: %v", err)
	}
}

// rejectCases 是「一次只改一处」的拒绝用例表；放在包级让测试函数体只留遍历。
var rejectCases = []struct {
	name   string
	mutate func(*Profile)
	code   Code
	want   string
}{
	{"空 id", func(p *Profile) { p.ID = "" }, CodeParam, "id 不能为空"},
	{"大写 id", func(p *Profile) { p.ID = "Sample" }, CodeParam, "只能包含小写字母"},
	{"空 name", func(p *Profile) { p.Name = "" }, CodeParam, "name 不能为空"},
	{"未知 schemaVersion", func(p *Profile) { p.SchemaVersion = 2 }, CodeParam, "只支持 1"},
	{"非法 kind", func(p *Profile) { p.Kind = "remote" }, CodeParam, "必须是 local 或 online"},
	{"非法扩展名", func(p *Profile) { p.Match.Ext = []string{"t x"} }, CodeParam, "match.ext 的第 1 项"},
	{"非法 glob", func(p *Profile) { p.Match.PathGlob = "/books/[" }, CodeParam, "不是合法的 glob"},
	{"坏 nameRegex", func(p *Profile) { p.Match.NameRegex = "^(" }, CodePattern, "match.nameRegex 的正则非法"},
	{"非法编码名", func(p *Profile) { p.Decode.ForceEncoding = "GBK!" }, CodeParam, "forceEncoding"},
	{"空禁用过滤器", func(p *Profile) { p.Filters.Disable = []string{" "} }, CodeParam, "filters.disable 的第 1 项"},
	{"空过滤规则名", func(p *Profile) { p.Filters.Extra[0].Name = "" }, CodeParam, "第 1 条过滤规则的名称不能为空"},
	{"坏过滤正则", func(p *Profile) { p.Filters.Extra[0].Pattern = "[" }, CodePattern, "第 1 条过滤规则的正则非法"},
	{"超长替换文本", func(p *Profile) { p.Filters.Extra[0].Replace = strings.Repeat("a", MaxPatternRunes+1) },
		CodeParam, "replace 不能超过 200 字"},
	{"规则总数超限", func(p *Profile) {
		p.Filters.Extra = nil
		for i := 0; i <= MaxRulesPerProfile; i++ {
			p.Filters.Extra = append(p.Filters.Extra, FilterRule{Name: fmt.Sprintf("r%d", i), Pattern: "^x$"})
		}
	}, CodeParam, "最多 30 条"},
	{"元数据无目标字段", func(p *Profile) { p.Meta[0].Author = 0 }, CodeParam, "至少要指定"},
	{"元数据捕获组越界", func(p *Profile) { p.Meta[0].Author = maxCaptureIndex + 1 }, CodeParam, "捕获组序号"},
	{"元数据空正则", func(p *Profile) { p.Meta[0].Pattern = " " }, CodeParam, "第 1 条元数据规则的正则不能为空"},
	{"非法强制策略名", func(p *Profile) { p.Split.ForceSplitter = "Di Unit" }, CodeParam, "forceSplitter"},
	{"第 2 条切章规则坏正则", func(p *Profile) {
		p.Split.Rules = append(p.Split.Rules, Rule{Name: "坏的", Pattern: "^("})
	}, CodePattern, "第 2 条切章规则的正则非法"},
	{"标签无关键词", func(p *Profile) { p.Tags[0].Keywords = nil }, CodeParam, "至少要有一个关键词"},
	{"同名标签", func(p *Profile) { p.Tags = append(p.Tags, TagRule{Tag: "奇幻", Keywords: []string{"精灵"}}) },
		CodeParam, "出现了两次"},
	{"关键词超长", func(p *Profile) { p.Tags[0].Keywords = []string{strings.Repeat("魔", MaxKeywordRunes+1)} },
		CodeParam, "关键词不能超过 20 字"},
	{"关键词过多", func(p *Profile) {
		kws := make([]string, MaxKeywords+1)
		for i := range kws {
			kws[i] = fmt.Sprintf("k%d", i)
		}
		p.Tags = []TagRule{{Tag: "奇幻", Keywords: kws}}
	}, CodeParam, "关键词不能超过 100 个"},
	{"负数 maxChapterChars", func(p *Profile) { p.Limits.MaxChapterChars = -1 }, CodeParam, "maxChapterChars"},
	{"超大 minChapters", func(p *Profile) { p.Limits.MinChapters = maxMinChapters + 1 }, CodeParam, "minChapters"},
	{"local 带 online 段", func(p *Profile) {
		p.Online = &Online{BookSourceURL: "https://example.invalid/x"}
	}, CodeParam, "不能带 online 段"},
	{"online 缺 URL", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{}
	}, CodeParam, "bookSourceUrl"},
	{"online 非 http", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{BookSourceURL: "ftp://example.invalid/x"}
	}, CodeParam, "必须以 http:// 或 https:// 开头"},
	{"online 分组超长", func(p *Profile) {
		p.Kind = KindOnline
		p.Online = &Online{BookSourceURL: "https://example.invalid/x", Group: strings.Repeat("组", MaxOnlineGroupRunes+1)}
	}, CodeParam, "online.group"},
	{"非文本书源", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{BookSourceURL: "https://example.invalid/x", BookSourceType: 1}
	}, CodeParam, "只支持文本书源"},
	{"搜索地址超长", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{
			BookSourceURL: "https://example.invalid/x", SearchURL: strings.Repeat("a", MaxOnlineURLRunes+1),
		}
	}, CodeParam, "searchUrl"},
	{"不支持字段名超长", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{
			BookSourceURL:     "https://example.invalid/x",
			UnsupportedFields: []string{strings.Repeat("a", MaxOnlineKeyRunes+1)},
		}
	}, CodeParam, "unsupportedFields"},
	{"header 键超长", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{
			BookSourceURL: "https://example.invalid/x",
			Header:        map[string]string{strings.Repeat("a", MaxOnlineKeyRunes+1): "v"},
		}
	}, CodeParam, "online.header"},
	{"header 值含 CR/LF", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{
			BookSourceURL: "https://example.invalid/x",
			Header:        map[string]string{"Referer": "a\r\nInjected: 1"},
		}
	}, CodeParam, "控制字符"},
	{"header 禁用 Host 键", func(p *Profile) {
		p.Kind, p.Online = KindOnline, &Online{
			BookSourceURL: "https://example.invalid/x",
			Header:        map[string]string{"Host": "evil.example"},
		}
	}, CodeParam, "Host"},
	{"online 规则块超限", func(p *Profile) {
		rules := make(map[string]string, MaxOnlineRuleKeys+1)
		for i := 0; i <= MaxOnlineRuleKeys; i++ {
			rules[fmt.Sprintf("k%d", i)] = "v"
		}
		p.Kind, p.Online = KindOnline, &Online{BookSourceURL: "https://example.invalid/x", RuleToc: rules}
	}, CodeParam, "最多 50 条规则"},
}

// TestValidateRejects 逐项拒绝并检查错误码与「第几条」定位。
func TestValidateRejects(t *testing.T) {
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProfile()
			tc.mutate(&p)
			err := p.Validate()
			if err == nil {
				t.Fatalf("应被拒绝")
			}
			if err.Code != tc.code {
				t.Errorf("错误码 = %q，期望 %q（消息：%s）", err.Code, tc.code, err.Message)
			}
			if !strings.Contains(err.Message, tc.want) {
				t.Errorf("消息 %q 不含 %q", err.Message, tc.want)
			}
		})
	}
}

// manyProfiles 生成 n 份 id 互不相同的合法规则包。
func manyProfiles(n int) []Profile {
	out := make([]Profile, 0, n)
	for i := 0; i < n; i++ {
		p := validProfile()
		p.ID = fmt.Sprintf("p-%d", i)
		out = append(out, p)
	}
	return out
}

// TestCheckAllEmpty 空批通过，逐条结果为空切片。
func TestCheckAllEmpty(t *testing.T) {
	issues, err := CheckAll(nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("空批 = (%v, %v)，期望空结果且无错误", issues, err)
	}
}

// TestCheckAllTooMany 超过 20 份整批拒绝：issues 为 nil（没有第几份可定位）。
func TestCheckAllTooMany(t *testing.T) {
	issues, err := CheckAll(manyProfiles(MaxProfiles + 1))
	if err == nil || issues != nil {
		t.Fatalf("超过 %d 份应整批拒绝: issues=%v err=%v", MaxProfiles, issues, err)
	}
	if err.Code != CodeParam {
		t.Errorf("错误码 = %q，期望 %q", err.Code, CodeParam)
	}
}

// TestCheckAllDuplicateID 重复 id 只标红后出现的那一份。
func TestCheckAllDuplicateID(t *testing.T) {
	issues, err := CheckAll([]Profile{validProfile(), validProfile()})
	if err == nil {
		t.Fatal("重复 id 应报错")
	}
	if len(issues) != 2 || !issues[0].OK || issues[1].OK {
		t.Fatalf("重复 id 应只标红第 2 份: %+v", issues)
	}
	if !strings.Contains(err.Message, "第 2 份规则包") {
		t.Errorf("消息 %q 未定位到第 2 份", err.Message)
	}
}

// TestCheckAllBadPattern 坏正则：错误码 invalid_pattern，逐条原因定位到具体规则。
func TestCheckAllBadPattern(t *testing.T) {
	bad := []Profile{validProfile(), validProfile()}
	bad[1].ID = "sample-2"
	bad[1].Split.Rules[0].Pattern = "^("
	issues, err := CheckAll(bad)
	if err == nil || err.Code != CodePattern {
		t.Fatalf("坏正则错误码 = %v，期望 %q", err, CodePattern)
	}
	if !issues[0].OK || issues[1].OK {
		t.Fatalf("只有第 2 份应标红: %+v", issues)
	}
	if issues[0].Index != 0 || issues[1].Index != 1 || issues[1].ID != "sample-2" {
		t.Errorf("逐条结果位置/身份不对: %+v", issues)
	}
	if !strings.Contains(issues[1].Reason, "第 1 条切章规则") {
		t.Errorf("原因 %q 未定位到具体规则", issues[1].Reason)
	}
}
