package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// 本文件锁死「正文压缩块损坏」的对外表现。
//
// 为什么单独测：块损坏是**局部故障**。几万本书的库里，一个坏块如果被映射成 500
// 「内部错误」，用户会以为整个库坏了、整本书打不开；正确表现是：
//   - HTTP 503 + 独立错误码 content_corrupted（前端可只在当前章提示）；
//   - 消息里带能定位的 book_id / block_seq；
//   - 书架与目录照常（它们不碰 packed）。

// corruptAllBlocks 把库里所有压缩块换成解压必然失败的字节。
func (e *testEnv) corruptAllBlocks() {
	e.t.Helper()
	if _, err := e.st.DB().ExecContext(context.Background(),
		"UPDATE chapter_blocks SET packed = x'02010203'"); err != nil {
		e.t.Fatalf("破坏块: %v", err)
	}
}

// TestGetChapterCorruptBlockReturns503 读坏块返回 503 + content_corrupted + 定位信息。
func TestGetChapterCorruptBlockReturns503(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("长夜书简", "青川客")
	env.corruptAllBlocks()

	rr := env.get(chapterPath(env, bookID))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，期望 503，body=%s", rr.Code, rr.Body.String())
	}
	body := decodeBody[errResp](t, rr)
	if body.Error.Code != codeContentCorrupted {
		t.Fatalf("错误码 = %q，期望 %q", body.Error.Code, codeContentCorrupted)
	}
	// 消息要能让用户/运维定位，但不能暴露库内路径。
	for _, want := range []string{"压缩块损坏", "book_id=", "block_seq="} {
		if !strings.Contains(body.Error.Message, want) {
			t.Errorf("错误消息缺少 %q：%s", want, body.Error.Message)
		}
	}
	if strings.Contains(body.Error.Message, "/tmp") || strings.Contains(body.Error.Message, "norvels.db") {
		t.Errorf("错误消息泄漏了库内路径：%s", body.Error.Message)
	}
}

// TestGetChapterCorruptBlockKeepsShelfAndTOC 坏块不影响书架与目录（局部故障）。
func TestGetChapterCorruptBlockKeepsShelfAndTOC(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("长夜书简", "青川客")
	env.corruptAllBlocks()

	t.Run("书架照常", func(t *testing.T) {
		rr := env.get("/api/books")
		if rr.Code != http.StatusOK {
			t.Fatalf("书架状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("目录照常", func(t *testing.T) {
		rr := env.get("/api/books/" + itoa64(bookID))
		if rr.Code != http.StatusOK {
			t.Fatalf("详情状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("章节列表照常", func(t *testing.T) {
		rr := env.get("/api/books/" + itoa64(bookID) + "/chapters")
		if rr.Code != http.StatusOK {
			t.Fatalf("章节列表状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
		}
	})
}

// TestGetChapterHealthyStillOK 正常章节不受影响（对照组，防止上面的测试假阳）。
func TestGetChapterHealthyStillOK(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("长夜书简", "青川客")

	rr := env.get(chapterPath(env, bookID))
	if rr.Code != http.StatusOK {
		t.Fatalf("正常章节状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "序章正文") {
		t.Errorf("正文内容缺失：%s", rr.Body.String())
	}
}

// chapterPath 取这本书的第一章 URL。
func chapterPath(env *testEnv, bookID int64) string {
	env.t.Helper()
	rr := env.get("/api/books/" + itoa64(bookID) + "/chapters")
	if rr.Code != http.StatusOK {
		env.t.Fatalf("取章节列表失败: %d", rr.Code)
	}
	page := decodeBody[struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}](env.t, rr)
	if len(page.Items) == 0 {
		env.t.Fatal("章节列表为空")
	}
	return "/api/books/" + itoa64(bookID) + "/chapters/" + itoa64(page.Items[0].ID)
}

// itoa64 是测试内的 int64 转字符串。
func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
