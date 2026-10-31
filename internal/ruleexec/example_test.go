package ruleexec_test

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"norvels/internal/ruleexec"
)

// Example 演示「一段 HTML → bookList 字段表」：先用列表规则选中条目，
// 再在每个条目上下文里取字段。
func Example() {
	html := `<ul class="book-list">
  <li><a class="name" href="/book/1">示例书一</a><span class="author">作者甲</span></li>
  <li><a class="name" href="/book/2">示例书二</a><span class="author">作者乙</span></li>
</ul>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		panic(err)
	}
	ctx := &ruleexec.Context{HTML: doc, Vars: map[string]string{"baseUrl": "https://example.invalid"}}

	list, err := ruleexec.Compile(".book-list li")
	if err != nil {
		panic(err)
	}
	nameRule, _ := ruleexec.Compile(".name@text")
	authorRule, _ := ruleexec.Compile(".author@text")
	urlRule, _ := ruleexec.Compile(".name@href | abs()")

	items, err := list.SelectAll(ctx)
	if err != nil {
		panic(err)
	}
	for _, item := range items {
		name, _ := nameRule.Eval(item)
		author, _ := authorRule.Eval(item)
		url, _ := urlRule.Eval(item)
		fmt.Printf("%s\t%s\t%s\n", name, author, url)
	}
	// Output:
	// 示例书一	作者甲	https://example.invalid/book/1
	// 示例书二	作者乙	https://example.invalid/book/2
}
