package api

import (
	"net/http"
	"strings"
	"testing"
)

// authorsBody 拼一个只含 authors 的 PATCH 请求体（避免测试里手写转义）。
func authorsBody(names ...string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "\""+n+"\"")
	}
	return "{\"authors\":[" + strings.Join(quoted, ",") + "]}"
}

// TestPatchBookAuthors PATCH authors 全量替换，主作者同步成 authors[0]。
func TestPatchBookAuthors(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("多作者补丁示例书", "原示例作者")
	target := bookPath(id)

	rr := env.patch(target, authorsBody("作者甲", "作者乙", "作者丙"))
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[bookDetailResp](t, rr)
	requireAuthors(t, body.Authors, "作者甲", "作者乙", "作者丙")
	if body.AuthorName != "作者甲" {
		t.Fatalf("authorName = %q，期望 authors[0]", body.AuthorName)
	}

	got := decodeBody[bookDetailResp](t, env.get(target))
	requireAuthors(t, got.Authors, "作者甲", "作者乙", "作者丙")
	if got.AuthorName != "作者甲" {
		t.Fatalf("GET 详情 authorName = %q，期望 authors[0]", got.AuthorName)
	}
	// 聚合口径不变：这本书只计到主作者名下，其余作者作品数为 0。
	requireAuthorAggregate(t, env, "作者甲")
}

// requireAuthors 断言 authors 列表逐项相等（顺序即语义）。
func requireAuthors(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("authors = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("authors[%d] = %q，期望 %q（顺序即语义）", i, got[i], want[i])
		}
	}
}

// requireAuthorAggregate 断言 /api/authors 的聚合口径：只有主作者记 1 本，其余 0。
func requireAuthorAggregate(t *testing.T, env *testEnv, primary string) {
	t.Helper()
	authors := decodeBody[authorsResp](t, env.get("/api/authors?size=50"))
	counts := make(map[string]int, len(authors.Items))
	for _, a := range authors.Items {
		counts[a.Name] = a.BookCount
	}
	if counts[primary] != 1 || counts["作者乙"] != 0 || counts["原示例作者"] != 0 {
		t.Fatalf("作者聚合 = %v，期望 %s 1 / 其余 0", counts, primary)
	}
}

// TestPatchBookAuthorsReplaceShrinks 再次 PATCH 只留一个作者时旧关联行要被清掉。
func TestPatchBookAuthorsReplaceShrinks(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("缩减作者示例书", "原示例作者")
	target := bookPath(id)

	if rr := env.patch(target, authorsBody("作者甲", "作者乙")); rr.Code != http.StatusOK {
		t.Fatalf("首次 PATCH 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	rr := env.patch(target, authorsBody("作者丙"))
	if rr.Code != http.StatusOK {
		t.Fatalf("二次 PATCH 状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[bookDetailResp](t, rr)
	if len(body.Authors) != 1 || body.Authors[0] != "作者丙" || body.AuthorName != "作者丙" {
		t.Fatalf("authors/authorName = %v/%q，期望 [作者丙]/作者丙", body.Authors, body.AuthorName)
	}
}

// TestPatchBookAuthorAndAuthorsPrefersAuthors 同时传 author 与 authors 不报错，authors 生效。
func TestPatchBookAuthorAndAuthorsPrefersAuthors(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("冲突字段示例书", "原示例作者")

	body := "{\"author\":\"作者丁\",\"authors\":[\"作者甲\",\"作者乙\"]}"
	rr := env.patch(bookPath(id), body)
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[bookDetailResp](t, rr)
	if got.AuthorName != "作者甲" || len(got.Authors) != 2 {
		t.Fatalf("authors/authorName = %v/%q，期望 authors 优先", got.Authors, got.AuthorName)
	}
}

// TestPatchBookAuthorsValidation authors 的跨信任边界校验（空数组 / 条数 / 单名长度）。
func TestPatchBookAuthorsValidation(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("作者校验示例书", "原示例作者")
	target := bookPath(id)

	cases := []struct {
		name string
		body string
	}{
		{"空数组", authorsBody()},
		{"只有空白名", authorsBody("  ", "")},
		{"超过十个", authorsBody("作者1", "作者2", "作者3", "作者4", "作者5",
			"作者6", "作者7", "作者8", "作者9", "作者10", "作者11")},
		{"单名过长", authorsBody(strings.Repeat("长", 101))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, env.patch(target, tc.body), http.StatusBadRequest, codeInvalidParam)
		})
	}

	// 拒掉的补丁不能改动库：作者仍是导入时的那一个。
	detail := decodeBody[bookDetailResp](t, env.get(target))
	if len(detail.Authors) != 1 || detail.Authors[0] != "原示例作者" {
		t.Fatalf("校验失败后 authors = %v，期望未改动", detail.Authors)
	}
}

// TestPatchBookClearsIntroSeriesAndIndex 传空串/0 即清空（不是「不改」）。
func TestPatchBookClearsIntroSeriesAndIndex(t *testing.T) {
	env := newEnv(t)
	id := env.seedBook("清空示例书", "示例作者")
	target := bookPath(id)

	full := "{\"intro\":\"原简介\",\"series\":\"原系列\",\"seriesIndex\":3}"
	if rr := env.patch(target, full); rr.Code != http.StatusOK {
		t.Fatalf("写入状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	clear := "{\"intro\":\"\",\"series\":\"\",\"seriesIndex\":0}"
	rr := env.patch(target, clear)
	if rr.Code != http.StatusOK {
		t.Fatalf("清空状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[bookDetailResp](t, rr)
	if body.Intro != "" || body.Series != "" || body.SeriesIndex != 0 {
		t.Fatalf("清空后 = intro %q series %q seriesIndex %d", body.Intro, body.Series, body.SeriesIndex)
	}

	got := decodeBody[bookDetailResp](t, env.get(target))
	if got.Intro != "" || got.Series != "" || got.SeriesIndex != 0 {
		t.Fatalf("GET 详情未清空：intro %q series %q seriesIndex %d", got.Intro, got.Series, got.SeriesIndex)
	}
}
