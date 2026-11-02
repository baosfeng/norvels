package api

import (
	"errors"
	"net/http"
	"strings"

	"norvels/internal/store"
)

// 账号（迁移 0016）：一个入口「输账号名就进」，没有密码。
//
// ---- 单入口口径 ----
// 前端只保留一个动作（「进入」→ POST /api/auth/enter）：账号不存在就建号并登录，
// 存在就直接登录。注册与登录不再由用户区分——「这个名字有没有人用」是后端的知识，
// 让用户去猜、猜错了再给个「换个按钮」的引导，是把内部实现泄漏成交互步骤。
// /register 与 /login 保留：它们是既有的稳定契约（部署脚本、验收场景在用），
// 且两者语义在单入口下依然清晰（不是就不建 / 建失败就报错）。
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

// userNameRequest 是 POST /api/auth/enter、/register 与 /login 的请求体。
type userNameRequest struct {
	Name string `json:"name"`
}

// handleEnter 处理 POST /api/auth/enter：单入口——不存在就建号并登录，存在就直接登录。
//
// 201（新建）/ 200（已存在）都下发会话 Cookie，前端不必知道走的是哪条路，
// 只有 toast 文案会按状态码区分「新建了」与「进来了」。
//
// 唯一的失败是「建号失败」（重名竞态 / 名字不合规 / 库错），此时走 failUserWrite：
// 409 / 400 / 500，与注册端点同一套映射，不另立一套口径。
func (s *Server) handleEnter(w http.ResponseWriter, r *http.Request) {
	name, ok := s.userNameFromBody(w, r)
	if !ok {
		return
	}
	u, created, err := s.st.EnterUser(r.Context(), name)
	if err != nil {
		s.failUserWrite(w, r, err, "进入失败")
		return
	}
	if u, ok = s.enterAs(w, r, u.ID); !ok {
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, u)
		return
	}
	writeJSON(w, http.StatusOK, u)
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
	if u, ok = s.enterAs(w, r, u.ID); !ok {
		return
	}
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
	if u, ok = s.enterAs(w, r, u.ID); !ok {
		return
	}
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

// handleDeleteUser 处理 DELETE /api/auth/users/{id}：删账号并清走它的个人数据，204。
//
// 三条拒绝口径（都不删任何东西）：
//   - 未登录（开放模式外的无会话请求）→ 401 unauthorized：删除是不可逆写操作，
//     不能让「门外的人」按 id 逐个试出库里有几个账号并删掉；
//   - id=1 默认账号 → 409 default_account：存量数据归属与开放模式兜底，删了库不自洽；
//   - 账号不存在 → 404 not_found。
//
// 删除当前账号时顺手撤掉本请求带来的会话 Cookie：客户端拿到 204 直接回未登录态，
// 不用再打一次 logout。
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.loggedIn(r) {
		writeError(w, http.StatusUnauthorized, codeUnauthorized, "需要登录后才能删除账号")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		s.badRequest(w, err, codeInvalidID)
		return
	}
	switch err := s.st.DeleteUser(r.Context(), id); {
	case errors.Is(err, store.ErrDefaultUser):
		writeError(w, http.StatusConflict, codeDefaultAccount, "默认账号（书库底座）不能删除")
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "账号不存在")
		return
	case err != nil:
		s.fail(w, r, err, "删除账号失败")
		return
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

// enterAs 是「这个账号现在进来了」的唯一落地口：刷新活跃时间 → 建会话 → 下发 Cookie。
//
// 把两件事绑在一起而不是让每个 handler 各写一遍：注册即登录、单入口、以后新增的
// 入口都必须同时产生「会话」与「活跃时间」，漏掉任何一个都会让账号列表显示
// 「从未登录」——这正是修前的 bug（注册只建了会话，没人刷 last_seen_at）。
//
// 返回落库后的账号（lastSeenAt 已是本次时间），调用方直接写进响应体。
func (s *Server) enterAs(w http.ResponseWriter, r *http.Request, id int64) (store.User, bool) {
	u, err := s.st.TouchUser(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, "记录登录失败")
		return store.User{}, false
	}
	s.startUserSession(w, r, u)
	return u, true
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

// loggedIn 报告这次请求是否带着一个**有效的账号会话**。
//
// 判定只看会话 Cookie 能不能换出账号，不看「能不能过鉴权」：开放模式（未配口令）
// 下任何请求都算过了鉴权，但那不代表有人登录——那种部署里 me() 回的是默认账号 reader。
// 删除账号这类不可逆写操作必须区分这两件事。
func (s *Server) loggedIn(r *http.Request) bool {
	tok := sessionToken(r)
	if tok == "" {
		return false
	}
	_, err := s.st.UserBySessionToken(r.Context(), tok)
	return err == nil
}

// failUserWrite 把账号写失败翻译成 409（重名）/400（入参）/500。
func (s *Server) failUserWrite(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, codeConflict, "这个账号名已经有人用了，换一个吧")
	case errors.Is(err, store.ErrInvalidArgument):
		s.failParam(w, err)
	case errors.Is(err, store.ErrDefaultUser):
		writeError(w, http.StatusConflict, codeDefaultAccount, "默认账号（书库底座）不能删除")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, codeAccountNotFound, "账号不存在，请先注册")
	default:
		s.fail(w, r, err, fallback)
	}
}
