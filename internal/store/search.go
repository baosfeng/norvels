package store

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// minSearchRunes 是可检索的最小字符数。单字在标题上命中范围过宽
// （「的」「一」几乎命中全书），对读者没有区分度，因此直接拒绝。
const minSearchRunes = 2

// 搜索关键词的拒绝提示（搜不出结果时直接告诉用户原因，不跑全表 LIKE）。
const (
	noticeEmptyQuery   = "请输入搜索关键词"
	noticeTooShort     = "关键词至少 2 个字"
	noticeNoSearchChar = "关键词需要包含文字或数字"
)

// hasSearchToken 判断关键词是否含可用于 LIKE 的字符（字母/数字/CJK）。
// 纯标点或空白的查询直接短路，避免构造出 "%%。" 这类必然命中又毫无意义的模式。
func hasSearchToken(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Search 按「书名 / 作者」检索。
//
// 检索范围只有**书籍元数据**：正文只做阅读，不做倒排索引；章节标题（目录）
// 属于一本书的内部结构，由详情页展开，搜索只回答「哪本书」。
// 命中走 bookFilter 的 LIKE（books.title / authors.name），没有索引可走，
// 但匹配的是千级 books 行，典型查询在毫秒级完成。
func (s *Store) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	page, size := normPage(q.Page, q.Size)
	query := strings.TrimSpace(q.Q)
	out := newEmptySearchResult(query, page, size)

	if notice := validateSearchQuery(query); notice != "" {
		out.Notice = notice
		return out, nil
	}

	books, err := s.ListBooks(ctx, bookQueryOf(q, out.Q, page, size))
	if err != nil {
		return nil, err
	}
	out.Books = books
	return out, nil
}

// bookQueryOf 把检索请求翻成书籍列表查询：筛选维度一对一透传，排序固定「最近更新」。
//
// 搜索不另写 WHERE：分类（只给 L2 时按不过滤）、评分、阅读状态、连载状态、作者、标签
// 的编译逻辑都在 bookFilter 里，复制一份就会和书籍列表页产生两套口径。
func bookQueryOf(q SearchQuery, keyword string, page, size int) BookQuery {
	return BookQuery{
		Q:            keyword,
		Page:         page,
		Size:         size,
		Sort:         "updated",
		CategoryL1:   q.CategoryL1,
		CategoryL2:   q.CategoryL2,
		Rating:       q.Rating,
		ReadStatus:   q.ReadStatus,
		SerialStatus: q.SerialStatus,
		AuthorID:     q.AuthorID,
		Tag:          q.Tag,
	}
}

// newEmptySearchResult 构造空结果骨架（分页字段先填好，命中数为 0）。
func newEmptySearchResult(query string, page, size int) *SearchResult {
	return &SearchResult{
		Q:     query,
		Books: Page[BookSummary]{Items: []BookSummary{}, Total: 0, Page: page, Size: size},
	}
}

// validateSearchQuery 检查关键词是否可检索；返回空串表示通过，否则是给前端的提示语。
func validateSearchQuery(query string) string {
	switch {
	case query == "":
		return noticeEmptyQuery
	case utf8.RuneCountInString(query) < minSearchRunes:
		return noticeTooShort
	case !hasSearchToken(query):
		return noticeNoSearchChar
	}
	return ""
}
