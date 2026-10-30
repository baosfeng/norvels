package api

import (
	"net/http"
	"strconv"
	"testing"
)

// 书架端点的口径（0016）：
//   - PUT / DELETE 幂等；
//   - 按账号隔离：两个会话各看各的；
//   - GET /api/shelf/ids 一次返回全量 id。

// shelfIDsBody 是书架 id 列表的响应结构。
type shelfIDsBody struct {
	BookIDs []int64 `json:"bookIds"`
}

// shelfIDs 取当前会话的书架 id 列表。
func shelfIDs(t *testing.T, e *testEnv, hdr map[string]string) []int64 {
	t.Helper()
	rr := e.do(http.MethodGet, "/api/shelf/ids", nil, hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/shelf/ids = %d，body=%s", rr.Code, rr.Body.String())
	}
	return decodeBody[shelfIDsBody](t, rr).BookIDs
}

// shelfPath 拼书架路径。
func shelfPath(bookID int64) string { return "/api/shelf/" + strconv.FormatInt(bookID, 10) }

// putShelf 加入书架并要求 204。
func putShelf(t *testing.T, e *testEnv, bookID int64, hdr map[string]string) {
	t.Helper()
	if rr := e.do(http.MethodPut, shelfPath(bookID), nil, hdr); rr.Code != http.StatusNoContent {
		t.Fatalf("加入书架 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
}

// delShelf 移出书架并要求 204。
func delShelf(t *testing.T, e *testEnv, bookID int64, hdr map[string]string) {
	t.Helper()
	if rr := e.do(http.MethodDelete, shelfPath(bookID), nil, hdr); rr.Code != http.StatusNoContent {
		t.Fatalf("移出书架 = %d，期望 204，body=%s", rr.Code, rr.Body.String())
	}
}

func TestShelfAddListRemove(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("示例书甲", "作者甲")
	token, _ := registerUser(t, e, "读者甲")
	hdr := withSession(token)

	putShelf(t, e, bookID, hdr)
	putShelf(t, e, bookID, hdr) // 幂等：重复加入不报错
	assertShelfHasOne(t, e, hdr, bookID)

	delShelf(t, e, bookID, hdr)
	delShelf(t, e, bookID, hdr) // 幂等：重复移出不报错
	if ids := shelfIDs(t, e, hdr); len(ids) != 0 {
		t.Errorf("移出后书架 ids = %v，期望空", ids)
	}
}

// assertShelfHasOne 校验书架 ids 与 /api/shelf 列表都只有这一本书。
func assertShelfHasOne(t *testing.T, e *testEnv, hdr map[string]string, bookID int64) {
	t.Helper()
	if ids := shelfIDs(t, e, hdr); len(ids) != 1 || ids[0] != bookID {
		t.Fatalf("书架 ids = %v，期望 [%d]", ids, bookID)
	}
	list := decodeBody[shelfListBody](t, e.do(http.MethodGet, "/api/shelf?page=1&size=10", nil, hdr))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != bookID {
		t.Fatalf("书架列表 = %+v，期望 1 本", list)
	}
	if list.Items[0].AddedAt == "" {
		t.Error("书架行缺 addedAt")
	}
}

// shelfListBody 是 GET /api/shelf 的断言用结构。
type shelfListBody struct {
	Items []struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		AddedAt string `json:"addedAt"`
	} `json:"items"`
	Total int `json:"total"`
}

// TestShelfIsolatedBetweenSessions 两个账号的书架互不可见。
func TestShelfIsolatedBetweenSessions(t *testing.T) {
	e := newEnv(t)
	bookID := e.seedBook("示例书甲", "作者甲")
	tokA, _ := registerUser(t, e, "读者甲")
	tokB, _ := registerUser(t, e, "读者乙")

	putShelf(t, e, bookID, withSession(tokA))
	if ids := shelfIDs(t, e, withSession(tokB)); len(ids) != 0 {
		t.Fatalf("乙不该看到甲的书架: %v", ids)
	}
	putShelf(t, e, bookID, withSession(tokB))
	if ids := shelfIDs(t, e, withSession(tokA)); len(ids) != 1 {
		t.Errorf("甲的书架 = %v，期望仍只有 1 本", ids)
	}
}

func TestShelfValidation(t *testing.T) {
	e := newEnv(t)
	token, _ := registerUser(t, e, "读者甲")
	hdr := withSession(token)
	for _, p := range []string{"/api/shelf/0", "/api/shelf/-1", "/api/shelf/abc"} {
		requireError(t, e.do(http.MethodPut, p, nil, hdr), http.StatusBadRequest, codeInvalidID)
	}
	requireError(t, e.do(http.MethodPut, "/api/shelf/999999", nil, hdr), http.StatusNotFound, codeNotFound)
}
