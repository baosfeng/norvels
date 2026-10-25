package api

import (
	"strings"
	"time"
	"unicode/utf8"

	"norvels/internal/store"
)

// bookPatch 是 PATCH /api/books/{id} 的请求体：任意字段子集，nil 表示「不改」。
//
// 未知字段按 encoding/json 默认行为忽略；但「只含未知字段」的 body 会归一化成空
// patch，最终按 400 invalid_param 拒绝——否则用户拼错字段名会得到一次静默成功。
type bookPatch struct {
	Title *string `json:"title"`
	// Author 与 Authors 同时给出时 Authors 优先（前端详情页可能同时持有两者）；
	// 只给 Author 时等价于 authors=[author]——两者都走「全量替换 books_authors」。
	Author      *string   `json:"author"`
	Authors     *[]string `json:"authors"`
	Intro       *string   `json:"intro"`
	Tags        *[]string `json:"tags"`
	Series      *string   `json:"series"`
	SeriesIndex *int      `json:"seriesIndex"`
	// SerialStatus 是连载状态（''|ongoing|completed），与解析状态 status 无关。
	SerialStatus *string `json:"serialStatus"`
	// ReadStatus 是阅读状态（''|want|reading|done），与 status / serialStatus 互不覆写。
	ReadStatus *string `json:"readStatus"`
	// Rating 是评分 0–5（0 = 未评）：0 不参与 rating 筛选与 sort=rating。
	Rating *int `json:"rating"`
	// 出版信息（P3 管理侧补全）：给出即全量写入，空串表示清空；
	// pubDate 只接受 YYYY-MM-DD，其余格式 400。
	Publisher *string `json:"publisher"`
	PubDate   *string `json:"pubDate"`
	ISBN      *string `json:"isbn"`
	Language  *string `json:"language"`
}

// bookPatchFieldsHint 是空 patch 的 400 文案（单本与批量共用，避免两处口径漂移）。
const bookPatchFieldsHint = "请求体至少包含一个可修改字段：" +
	"title/author/authors/intro/tags/series/seriesIndex/serialStatus/readStatus/" +
	"rating/publisher/pubDate/isbn/language"

// PATCH authors 的跨信任边界上限：条数与单名长度。
const (
	maxPatchAuthors    = 10
	maxAuthorNameRunes = 100
)

// validateAuthors 校验 PATCH 带来的作者列表：条数与单名长度受限；空数组合法（= 清空作者）。
//
// 「什么算同一个作者」由 store.NormalizeAuthors 单点定义（去空白、去空名、保序去重）。
// 空数组不再 400（P1-B §2.5 口径覆盖）：前端批量设作者时留空即清空，下发 [] 应当成功。
func validateAuthors(authors *[]string) error {
	if authors == nil {
		return nil
	}
	names := store.NormalizeAuthors(*authors)
	if len(names) > maxPatchAuthors {
		return badParam("authors 最多 %d 个作者", maxPatchAuthors)
	}
	for _, name := range names {
		if utf8.RuneCountInString(name) > maxAuthorNameRunes {
			return badParam("单个作者名不能超过 %d 字", maxAuthorNameRunes)
		}
	}
	return nil
}

// validateSerialStatus 校验 PATCH 带来的连载状态：” / ongoing / completed，其它值 400。
//
// 合法取值集合由 store.ValidSerialStatus 单点定义（写入侧还有一道同样的守卫），
// api 只负责把它翻译成 400 invalid_param。
func validateSerialStatus(status *string) error {
	if status == nil {
		return nil
	}
	if !store.ValidSerialStatus(strings.TrimSpace(*status)) {
		return badParam("serialStatus 只支持 ''（未知）/ ongoing / completed")
	}
	return nil
}

// validateReadStatus 校验 PATCH 带来的阅读状态：” / want / reading / done，其它值 400。
//
// 合法取值集合由 store.ValidReadStatus 单点定义（写入侧还有一道同样的守卫），
// api 只负责把它翻译成 400 invalid_param。
func validateReadStatus(status *string) error {
	if status == nil {
		return nil
	}
	if !store.ValidReadStatus(strings.TrimSpace(*status)) {
		return badParam("readStatus 只支持 ''（未标记）/ want / reading / done")
	}
	return nil
}

// validateRating 校验评分：0 = 未评，1–5 是有效星级，越界 400（不静默截断）。
func validateRating(rating *int) error {
	if rating == nil {
		return nil
	}
	if *rating < 0 || *rating > 5 {
		return badParam("rating 必须在 0~5 之间（0 表示未评）")
	}
	return nil
}

// validatePubDate 校验出版日期：空串合法（清空），其余必须是 YYYY-MM-DD。
func validatePubDate(pubDate *string) error {
	if pubDate == nil {
		return nil
	}
	date := strings.TrimSpace(*pubDate)
	if date == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return badParam("pubDate 必须是 YYYY-MM-DD（留空表示清空）")
	}
	return nil
}

// validateBookPatch 是单本 PATCH 与批量 PATCH 共用的字段校验：口径必须一致，
// 否则同一份 patch 单本成功、批量失败（或反过来）会让前端无从解释。
func validateBookPatch(p bookPatch) error {
	checks := []func() error{
		func() error { return validateAuthors(p.Authors) },
		func() error { return validateSerialStatus(p.SerialStatus) },
		func() error { return validateReadStatus(p.ReadStatus) },
		func() error { return validateRating(p.Rating) },
		func() error { return validatePubDate(p.PubDate) },
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// toStorePatch 把请求体转成库层补丁（两层的字段集合故意一一对应，转换是纯搬运）。
func (p bookPatch) toStorePatch() store.BookMetaPatch {
	return store.BookMetaPatch{
		Title:        p.Title,
		Author:       p.Author,
		Authors:      p.Authors,
		Intro:        p.Intro,
		Series:       p.Series,
		SeriesIndex:  p.SeriesIndex,
		Tags:         p.Tags,
		SerialStatus: p.SerialStatus,
		ReadStatus:   p.ReadStatus,
		Rating:       p.Rating,
		Publisher:    p.Publisher,
		PubDate:      p.PubDate,
		ISBN:         p.ISBN,
		Language:     p.Language,
	}
}
