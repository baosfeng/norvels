package crawl

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/enrich"
	"norvels/internal/source"
)

// legadoHTML 是简写规则要匹配的固定样例页：表头 + 一条书（书名/作者/简介/分类）。
const legadoHTML = "<table class='grid'>" +
	"<tr class='head'><td>书名</td><td>作者</td></tr>" +
	"<tr class='odd'>" +
	"<td><a href='/book/1'>示例书一</a></td>" +
	"<td>作者甲</td>" +
	"<td><div class='info'><h3>这是一段足够长的示例简介，用来验证简介字段能被正确取出并参与投票。</h3></div></td>" +
	"<td><div id='intro'><p>玄幻/东方玄幻</p><p>第二段</p></div></td>" +
	"</tr></table>"

// legadoSearchProfile 返回一份规则全用 Legado 简写写法的在线书源（模拟库里已有的旧数据）。
func legadoSearchProfile(port string) source.Profile {
	prof := profileFor("src-a", port)
	prof.Online.RuleSearch["bookList"] = "class.grid@tr!0"
	prof.Online.RuleSearch["name"] = "tag.td.0@tag.a@text"
	prof.Online.RuleSearch["author"] = "tag.td.1@text"
	prof.Online.RuleSearch["intro"] = "class.info@h3@text"
	prof.Online.RuleSearch["kind"] = "id.intro@tag.p.0@text"
	prof.Online.RuleSearch["bookUrl"] = "tag.td.0@tag.a@href | abs()"
	return prof
}

// TestSearchBookLegadoShorthand 旧规则（库里存的 Legado 简写）在加载时转换后能真正抽到数据。
func TestSearchBookLegadoShorthand(t *testing.T) {
	srv, _ := newSearchServer(t, legadoHTML)
	port, p := serverPort(t, srv)
	prof := legadoSearchProfile(port)
	e, err := NewEngine([]source.Profile{prof}, Options{QPS: -1, AllowedPorts: []int{p},
		DialContext: dialerFor(t, srv.Listener.Addr().String())})
	if err != nil {
		t.Fatal(err)
	}
	requireLoadTimeConversion(t, e, prof)
	res, err := e.SearchBook(context.Background(), "示例书一")
	if err != nil {
		t.Fatal(err)
	}
	requireLegadoHit(t, res.Hits)
}

// requireLoadTimeConversion 断言加载时转换生效、且不改写调用方传入的规则包。
func requireLoadTimeConversion(t *testing.T, e *Engine, prof source.Profile) {
	t.Helper()
	if got := e.sources[0].profile.Online.RuleSearch["bookList"]; got != ".grid tr:not(:nth-child(1))" {
		t.Errorf("加载时未转换 bookList：%q", got)
	}
	if got := prof.Online.RuleSearch["bookList"]; got != "class.grid@tr!0" {
		t.Errorf("转换不应修改调用方传入的规则包，实际 %q", got)
	}
}

// requireLegadoHit 断言固定 fixture 上书名 / 作者 / 简介 / 分类 / 链接都抽到了。
func requireLegadoHit(t *testing.T, hits []enrich.Hit) {
	t.Helper()
	if len(hits) != 1 {
		t.Fatalf("应命中 1 条，实际 %d：%+v", len(hits), hits)
	}
	h := hits[0]
	if h.Title != "示例书一" || h.Author != "作者甲" || h.Kind != "玄幻/东方玄幻" {
		t.Errorf("字段抽取不完整: %+v", h)
	}
	if !strings.Contains(h.Intro, "示例简介") {
		t.Errorf("intro 未抽出: %q", h.Intro)
	}
	if !strings.HasSuffix(h.BookURL, "/book/1") {
		t.Errorf("bookUrl 应解析成绝对地址，实际 %q", h.BookURL)
	}
}
