package api

import (
	"errors"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 账号（迁移 0016）：只注册 + 输账号登录，没有密码。
//
// ---- 与旧共享口令的关系 ----
// NORVELS_AUTH_TOKEN 的语义**完全不变**：配置了就仍要求 Bearer / 旧 Cookie，
// 部署脚本与健康检查照旧可用。新账号会话是**额外**的一层：
//   - 未配置 token（开放模式）：所有请求按默认账号（id=1）读写个人数据，
//     与单用户时代行为一致；
//   - 配置了 token：通过 token 校验后，若还带着有效的新会话 Cookie，个人数据
//     按该账号隔离；只带 Bearer 的管理侧调用回落到默认账号。
//
// 换句话说：账号体系统管「这些记录属于谁」，共享口令继续管「能不能进门」。

// 账号名与请求体的边界。
const maxUserNameRunes = 32

// userNameRequest 是 POST /api/auth/register 与 /api/auth/login 的请求体。
type userNameRequest struct {
	Name string `json:"name"`
}

// handleRegister 处理 POST /api/auth/register：201 + 账号，并直接下发会话。
//
// 注册即登录：用户刚输入完名字，再让他登录一次是纯粹的仪式感。
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	name, ok := s.userNameFromBody(w, r)
	if !ok {
		return
	}
	u, err := s.st.RegisterUser(r.Context(), name)
	if err != nil {
		s.failUserWrite(w, r, err, "注册失败")
		return
	}
	s.startUserSession(w, r, u)
	writeJSON(w, http.StatusCreated, u)
}

// handleLogin 处理 POST /api/auth/login：200 + 账号；账号不存在 404。
//
// 404 + 「账号不存在，请先注册」而不是 401：这里没有密码可输错，
// 唯一的失败原因就是号还没建，给出可执行的下一步比「未授权」有用。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	name, ok := s.userNameFromBody(w, r)
	if !ok {
		return
	}
	u, err := s.st.LoginUser(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, codeAccountNotFound, "账号不存在，请先注册")
			return
		}
		s.failUserWrite(w, r, err, "登录失败")
		return
	}
	s.startUserSession(w, r, u)
	writeJSON(w, http.StatusOK, u)
}

// handleAuthLogout 处理 POST /api/auth/logout：撤会话并清 Cookie，204。
//
// 幂等：没有会话 / Cookie 已失效同样 204——登出永远该成功，否则前端会被
// 「登不出去」卡住。
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if tok := sessionToken(r); tok != "" {
		if err := s.st.DeleteUserSession(r.Context(), tok); err != nil {
			s.logf("api: 撤销会话失败: %v", err)
		}
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthMe 处理 GET /api/auth/me：返回当前账号。
//
// 开放模式（未配置 token）下没有登录这回事，回默认账号——前端据此展示
// 「当前是谁」，行为与单用户时代一致。
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	id := store.ScopeUserID(r.Context())
	u, err := s.st.UserByID(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// userListItem 是账号列表的一行：字段与账号对象一致，只有 lastSeenAt 的
// 「没有时间」写成 null 而不是空串（列表要按时间展示，空串容易被当成坏数据）。
type userListItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	LastSeenAt *string `json:"lastSeenAt"`
}

// userListResponse 是 GET /api/auth/users 的响应：items 最多 50 条，total 是账号总数。
type userListResponse struct {
	Items []userListItem `json:"items"`
	Total int            `json:"total"`
}

// handleListUsers 处理 GET /api/auth/users：登录页「选已有账号 or 新建账号」的列表。
//
// ---- 为什么这个端点不鉴权 ----
// 它唯一的消费者是登录页，而登录页本身就在鉴权之外（/login 免鉴权）：
// 要求持凭证才能拿到账号名，等于要求先登录才能选账号登录，没有入口可用。
// 代价是未进门的人能看到「这台库上有哪些账号名」。本产品的部署前提是个人 /
// 家庭局域网内的私有书库（见 deploy/nas 与 ADR-0001），账号无密码、账号名
// 也不是秘密（它标识的是「这些阅读记录属于谁」），这个泄露面可以接受。
// 要暴露到公网请先换成真正的身份系统，而不是给这里补一层鉴权。
//
// 返回空列表时 items 是 []、不是 null：前端直接 map 不必判空。
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, total, err := s.st.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err, "账号列表不可用")
		return
	}
	items := make([]userListItem, 0, len(users))
	for _, u := range users {
		seen := u.LastSeenAt
		item := userListItem{ID: u.ID, Name: u.Name, CreatedAt: u.CreatedAt}
		if seen != "" {
			item.LastSeenAt = &seen
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, userListResponse{Items: items, Total: total})
}

// startUserSession 建会话并把 token 写进 HttpOnly Cookie。
func (s *Server) startUserSession(w http.ResponseWriter, r *http.Request, u store.User) {
	token, err := s.st.CreateUserSession(r.Context(), u.ID)
	if err != nil {
		s.logf("api: 建会话失败 user=%d: %v", u.ID, err)
		return
	}
	http.SetCookie(w, sessionCookie(r, token, int(store.UserSessionTTL.Seconds())))
}

// userNameFromBody 解析并校验账号名；出错时已写好 400。
func (s *Server) userNameFromBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body userNameRequest
	if !decodeJSONBody(w, r, &body, `请求体必须是 {"name":"账号名"}`) {
		return "", false
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "账号名不能为空")
		return "", false
	}
	if len([]rune(name)) > maxUserNameRunes {
		writeError(w, http.StatusBadRequest, codeInvalidParam, "账号名不能超过 32 字")
		return "", false
	}
	return name, true
}

// failUserWrite 把账号写失败翻译成 409（重名）/400（入参）/500。
func (s *Server) failUserWrite(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "这个账号名已经有人用了，换一个吧")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, codeAccountNotFound, "账号不存在，请先注册")
	default:
		s.fail(w, r, err, fallback)
	}
}
