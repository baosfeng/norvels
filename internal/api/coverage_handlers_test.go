package api

// 封面 / 账号 / 书架端点里尚未覆盖的失败分支。
// 这些都是「用户手滑或库被手改」时真正会走到的路径，不是凑数的空跑。

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"norvels/internal/store"
)

// multipartWriterForTest 造一个写内存的 multipart writer。
func multipartWriterForTest(buf *bytes.Buffer) *multipart.Writer { return multipart.NewWriter(buf) }

// emptyCoverMultipart 构造一个 cover 字段但内容为空的 multipart 请求体。
func emptyCoverMultipart(t *testing.T, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipartWriterForTest(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+coverFormField+`"; filename="cover.png"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(nil); err != nil {
		t.Fatalf("写入空 part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 multipart: %v", err)
	}
	return &buf, w.FormDataContentType()
}

// TestPutCoverEmptyFile 空文件在落库前就被挡回 400（不写 0 字节封面）。
func TestPutCoverEmptyFile(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("空封面示例书", "示例作者")
	body, contentType := emptyCoverMultipart(t, "image/png")
	rr := env.putCover(bookID, body, contentType)
	requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	requireErrMessage(t, rr, "空")
	// 没写进去：详情里的 hasCover 必须还是 false。
	if detail := decodeBody[bookDetailResp](t, env.get(bookPath(bookID))); detail.HasCover {
		t.Error("空封面不该落库")
	}
}

// TestPutCoverContentTypeSniffed 客户端不声明 Content-Type 时按内容嗅探出 PNG。
func TestPutCoverContentTypeSniffed(t *testing.T) {
	env := newEnv(t)
	bookID := env.seedBook("嗅探封面示例书", "示例作者")
	body, contentType := coverMultipart(t, coverFormField, "", pngBytes())
	if rr := env.putCover(bookID, body, contentType); rr.Code != http.StatusNoContent {
		t.Fatalf("上传状态码 = %d，body=%s", rr.Code, rr.Body.String())
	}
	got := env.get(bookPath(bookID) + "/cover")
	if ct := got.Header().Get("Content-Type"); !strings.Contains(ct, "png") {
		t.Errorf("嗅探出的 Content-Type = %q，期望 png", ct)
	}
}

// TestCoverHandlersBadID 路径参数非法：上传与删除封面都先判 id 再碰 store。
func TestCoverHandlersBadID(t *testing.T) {
	env := closedEnv(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rr := env.do(method, "/api/books/0/cover", nil, nil)
		requireError(t, rr, http.StatusBadRequest, codeInvalidID)
	}
}

// TestCoverParamMessage 入参错误按原因给出可读文案（不是笼统的「参数非法」）。
func TestCoverParamMessage(t *testing.T) {
	cases := map[string]string{
		"store: invalid argument: cover mime \"text/plain\"":     "JPEG",
		"store: invalid argument: cover 9 bytes exceeds 2097152": "2 MiB",
		"store: invalid argument: empty cover bytes":             "空",
		"store: invalid argument: 其它原因":                          "其它原因",
	}
	for msg, want := range cases {
		if got := coverParamMessage(errors.New(msg)); !strings.Contains(got, want) {
			t.Errorf("coverParamMessage(%q) = %q，期望包含 %q", msg, got, want)
		}
	}
}

// TestFailCoverWriteStoreDown store 不可用时封面写入是 500（不是 404「书不存在」）。
func TestFailCoverWriteStoreDown(t *testing.T) {
	rr := httptest.NewRecorder()
	closedEnv(t).srv.failCoverWrite(rr, httptest.NewRequest(http.MethodPut, "/api/books/1/cover", nil),
		fmt.Errorf("store: db is closed"))
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
}

// TestUserWriteErrors 账号写的四类 store 错误各自的 HTTP 映射。
func TestUserWriteErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"重名", fmt.Errorf("%w: 账号名已被占用", store.ErrConflict), http.StatusConflict, codeConflict},
		{"入参非法", fmt.Errorf("%w: 账号名不能为空", store.ErrInvalidArgument), http.StatusBadRequest, codeInvalidParam},
		{"账号不存在", fmt.Errorf("%w: 账号不存在", store.ErrNotFound), http.StatusNotFound, codeAccountNotFound},
		{"其它", errors.New("db down"), http.StatusInternalServerError, codeInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv := newEnv(t).srv
			srv.failUserWrite(rr, httptest.NewRequest(http.MethodPost, "/api/auth/register", nil), c.err, "注册失败")
			requireError(t, rr, c.want, c.code)
		})
	}
}

// TestAuthRegisterDuplicateName 重名注册是 409，且原有账号不受影响。
func TestAuthRegisterDuplicateName(t *testing.T) {
	e := newEnv(t)
	_, firstID := registerUser(t, e, "读者甲")
	rr := e.postJSON("/api/auth/register", nameJSON("读者甲"))
	requireError(t, rr, http.StatusConflict, codeConflict)
	// 原账号仍可登录，id 不变（重复注册不该覆盖既有账号）。
	login := e.postJSON("/api/auth/login", nameJSON("读者甲"))
	if login.Code != http.StatusOK {
		t.Fatalf("原账号登录 = %d，body=%s", login.Code, login.Body.String())
	}
	if got := decodeBody[authUser](t, login); got.ID != firstID {
		t.Errorf("账号 id = %d，期望仍是 %d", got.ID, firstID)
	}
}

// TestAuthNameValidation 账号名边界：空名 400、超 32 字 400。
func TestAuthNameValidation(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{nameJSON(""), nameJSON("   "), nameJSON(strings.Repeat("长", maxUserNameRunes+1))} {
		rr := e.postJSON("/api/auth/register", body)
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
}

// TestAuthLogoutWithoutSession 没有会话 Cookie 时登出仍 204 且清空 Cookie。
func TestAuthLogoutWithoutSession(t *testing.T) {
	e := newEnv(t)
	rr := e.do(http.MethodPost, "/api/auth/logout", nil, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("登出状态码 = %d，期望 204", rr.Code)
	}
	if c := findCookie(t, rr, SessionCookieName); c == nil || c.MaxAge >= 0 {
		t.Errorf("登出应下发已过期的会话 Cookie，实际 %+v", c)
	}
}

// TestShelfListEmptyShape 空书架返回 items:[] 而不是 null（前端直接 map）。
func TestShelfListEmptyShape(t *testing.T) {
	e := newEnv(t)
	rr := e.get("/api/shelf")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/shelf = %d，body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Errorf("空书架响应 = %s，期望 items 是 []", rr.Body.String())
	}
	ids := e.get("/api/shelf/ids")
	if !strings.Contains(ids.Body.String(), `"bookIds":[]`) {
		t.Errorf("空书架 ids = %s，期望 bookIds 是 []", ids.Body.String())
	}
}
