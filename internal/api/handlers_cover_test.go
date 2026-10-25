package api

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// 封面上传 / 删除的接口口径（管理页 M1）。

// pngBytes 是一份带 PNG 魔数的假图：store 只校验 MIME 与体积，不解码像素。
func pngBytes() []byte { return append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("x"), 64)...) }

// coverMultipart 构造一次 multipart 上传请求体。
func coverMultipart(t *testing.T, field, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="cover.png"`, field))
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("写入封面上传体: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 multipart: %v", err)
	}
	return &buf, w.FormDataContentType()
}

// putCover 发起一次封面上传。
func (e *testEnv) putCover(bookID int64, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPut, bookPath(bookID)+"/cover", body,
		map[string]string{"Content-Type": contentType})
}

// TestPutAndDeleteCover 上传→读图→删除→再删（幂等）的完整闭环。
func TestPutAndDeleteCover(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("带封面的示例书", "示例作者")
	image := pngBytes()

	body, contentType := coverMultipart(t, coverFormField, "image/png", image)
	if rr := env.putCover(bookID, body, contentType); rr.Code != http.StatusNoContent {
		t.Fatalf("上传状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := env.get(bookPath(bookID) + "/cover")
	if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), image) {
		t.Fatalf("读封面状态 = %d，字节数 = %d", got.Code, got.Body.Len())
	}
	if ct := got.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("封面 Content-Type = %q，期望 image/png", ct)
	}
	if detail := decodeBody[bookDetailResp](t, env.get(bookPath(bookID))); !detail.HasCover {
		t.Errorf("详情 hasCover = false，期望 true")
	}

	if rr := env.do(http.MethodDelete, bookPath(bookID)+"/cover", nil, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("删除状态码 = %d", rr.Code)
	}
	if rr := env.get(bookPath(bookID) + "/cover"); rr.Code != http.StatusNotFound {
		t.Errorf("删除后读封面状态码 = %d，期望 404", rr.Code)
	}
	if detail := decodeBody[bookDetailResp](t, env.get(bookPath(bookID))); detail.HasCover {
		t.Errorf("删除后 hasCover = true，期望 false")
	}
	if rr := env.do(http.MethodDelete, bookPath(bookID)+"/cover", nil, nil); rr.Code != http.StatusNoContent {
		t.Errorf("重复删除状态码 = %d，期望幂等的 204", rr.Code)
	}
}

// TestPutCoverRejects 拒绝非法上传：非图片、超大、缺字段、书不存在。
func TestPutCoverRejects(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("封面校验用示例书", "示例作者")

	textBody, textType := coverMultipart(t, coverFormField, "text/plain", []byte("这不是图片"))
	rr := env.putCover(bookID, textBody, textType)
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	requireErrMessage(t, rr, "JPEG")

	bigBody, bigType := coverMultipart(t, coverFormField, "image/png",
		append(pngBytes(), bytes.Repeat([]byte("x"), 3<<20)...))
	rr = env.putCover(bookID, bigBody, bigType)
	if rr.Code != http.StatusRequestEntityTooLarge && rr.Code != http.StatusBadRequest {
		t.Errorf("超大封面状态码 = %d，期望 413 或 400", rr.Code)
	}

	emptyBody, emptyType := coverMultipart(t, "wrong", "image/png", pngBytes())
	rr = env.putCover(bookID, emptyBody, emptyType)
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	requireErrMessage(t, rr, "cover")

	okBody, okType := coverMultipart(t, coverFormField, "image/png", pngBytes())
	rr = env.putCover(99999, okBody, okType)
	requireError(t, rr, http.StatusNotFound, codeNotFound)
}

// requireErrMessage 断言错误消息里含某个片段（码与状态码由 requireError 负责）。
func requireErrMessage(t *testing.T, rr *httptest.ResponseRecorder, fragment string) {
	t.Helper()
	payload := decodeBody[errResp](t, rr)
	if !strings.Contains(payload.Error.Message, fragment) {
		t.Errorf("错误消息 = %q，应含 %q", payload.Error.Message, fragment)
	}
}
