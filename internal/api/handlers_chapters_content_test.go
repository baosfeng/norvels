package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"norvels/internal/model"
	"norvels/internal/store"
)

// 整章替换正文（管理页 M1）：PUT /api/books/{id}/chapters/{cid}/content。
// 成功 204（正文与字数由前端重拉 GET 得到）；空正文 / 超 1 MiB → 400；
// 章节不属于本书或不存在 → 404。

// TestPutChapterContent 替换成功：204 无响应体 → 重拉 GET 得到逐字节一致的新正文与新字数。
func TestPutChapterContent(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	target := chapterURL(bookID, ids[0]) + "/content"

	replacement := "改过的正文。\n第二段。"
	rr := env.putJSON(target, map[string]string{"content": replacement})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，期望 204（body=%s）", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); body != "" {
		t.Errorf("204 不该有响应体，得到 %q", body)
	}
	got := decodeBody[store.ChapterContent](t, env.get(chapterURL(bookID, ids[0])))
	if got.Content != replacement || len(got.Content) != len(replacement) {
		t.Fatalf("重拉正文 = %q（%d 字节），期望 %q（%d 字节）",
			got.Content, len(got.Content), replacement, len(replacement))
	}
	if got.WordCount != 8 {
		t.Errorf("wordCount = %d，期望 8（改过的正文 5 字 + 第二段 3 字）", got.WordCount)
	}
	// 章 id / idx 不变：阅读进度与书签按 chapter id 指过来
	if got.ID != ids[0] || got.Idx != 0 {
		t.Errorf("id/idx = %d/%d，期望 %d/0", got.ID, got.Idx, ids[0])
	}
	// 目录字数与阅读页同源：重拉目录应看到同一个数
	if chapters := env.bookChapters(bookID); chapters.Items[0].WordCount != 8 {
		t.Errorf("目录首章字数 = %d，期望 8（替换不增删章，章数仍为 %d）",
			chapters.Items[0].WordCount, chapters.Total)
	}
}

// TestPutChapterContentErrors 错误码：空正文 / 超 1 MiB → 400 invalid_param，
// 章节不属于本书或不存在 → 404 not_found，路径 id 非数字 → 400 invalid_id。
func TestPutChapterContentErrors(t *testing.T) {
	env := newEnv(t)
	bookID, ids := env.seedParagraphBook()
	otherBook, otherIDs := env.seedContentBookAPI("api-content-other")
	_ = otherBook
	target := chapterURL(bookID, ids[0]) + "/content"
	over := strings.Repeat("字", 400000) // 1.2 MB，超 1 MiB 上限
	notFound := "{\"content\":\"新正文。\"}"

	cases := []struct {
		name   string
		target string
		body   string
		status int
		code   string
	}{
		{"空正文", target, "{\"content\":\"\"}", http.StatusBadRequest, codeInvalidParam},
		{"空白正文", target, "{\"content\":\"  \\n\\t \"}", http.StatusBadRequest, codeInvalidParam},
		{"超 1 MiB", target, "{\"content\":\"" + over + "\"}", http.StatusBadRequest, codeInvalidParam},
		{"缺 content 字段", target, "{}", http.StatusBadRequest, codeInvalidParam},
		{"非法 JSON", target, "{\"content\":", http.StatusBadRequest, codeInvalidParam},
		{"章节不属于本书", chapterURL(bookID, otherIDs[0]) + "/content", notFound, http.StatusNotFound, codeNotFound},
		{"章节不存在", chapterURL(bookID, 999999) + "/content", notFound, http.StatusNotFound, codeNotFound},
		{"书不存在", chapterURL(999999, ids[0]) + "/content", notFound, http.StatusNotFound, codeNotFound},
		{"章节 id 非法", "/api/books/" + strconv.FormatInt(bookID, 10) + "/chapters/abc/content",
			notFound, http.StatusBadRequest, codeInvalidID},
	}
	for _, tc := range cases {
		got := env.do(http.MethodPut, tc.target, strings.NewReader(tc.body),
			map[string]string{"Content-Type": "application/json"})
		if got.Code != tc.status {
			t.Errorf("%s：状态码 = %d，期望 %d（body=%s）", tc.name, got.Code, tc.status, got.Body.String())
			continue
		}
		if code := errorCodeOf(t, got); code != tc.code {
			t.Errorf("%s：错误码 = %q，期望 %q", tc.name, code, tc.code)
		}
	}
	// 超限的 message 要写清上限（人能看懂），不是笼统的「请求参数非法」
	msg := decodeBody[errorBody](t, env.putJSON(target, map[string]string{"content": over})).Error.Message
	if !strings.Contains(msg, "1048576") {
		t.Errorf("超限 message = %q，期望写清上限 1048576", msg)
	}
	// 全部被拒之后，原正文一个字节都没变，别的书也没被牵连
	ch := decodeBody[store.ChapterContent](t, env.get(chapterURL(bookID, ids[0])))
	if ch.Content != "第一段落。\n第二段落。\n第三段落。\n第四段落。" {
		t.Errorf("拒绝后正文 = %q，期望原正文", ch.Content)
	}
	other := decodeBody[store.ChapterContent](t, env.get(chapterURL(otherBook, otherIDs[0])))
	if other.Content != "别人的正文。" {
		t.Errorf("他人的章正文 = %q，期望原正文", other.Content)
	}
	t.Logf("超限 message：%s", msg)
}

// seedContentBookAPI 写一本单章书（sha 按 tag 区分，避免与 seedParagraphBook 撞路径/sha）。
func (e *testEnv) seedContentBookAPI(tag string) (int64, []int64) {
	e.t.Helper()
	sum := sha256.Sum256([]byte("norvels-api-test:" + tag))
	res, err := e.st.SaveBook(context.Background(), &model.ParsedBook{
		Title:      "正文接口样本 " + tag,
		Author:     "样本作者",
		SourcePath: "/books/" + tag + ".txt",
		SourceName: tag + ".txt",
		Encoding:   "utf-8",
		FileSize:   2048,
		SHA256:     hex.EncodeToString(sum[:]),
		Volumes:    []model.ParsedVolume{{Idx: 0, Title: "第一卷"}},
		Chapters: []model.ParsedChapter{
			{Idx: 0, VolumeIdx: 0, Title: "别人的章", Content: "别人的正文。", WordCount: 5},
		},
	}, store.SaveOptions{})
	if err != nil {
		e.t.Fatalf("SaveBook(%s): %v", tag, err)
	}
	list := e.bookChapters(res.BookID)
	ids := make([]int64, 0, len(list.Items))
	for _, item := range list.Items {
		ids = append(ids, item.ID)
	}
	return res.BookID, ids
}
