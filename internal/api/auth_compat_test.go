package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// jsonBody 把字符串包成带 JSON Content-Type 的请求体。
func jsonBody(s string) io.Reader { return strings.NewReader(s) }

// 向后兼容的口径（0016）：账号体系是**额外**的一层，不能破坏现有部署。
//   - 开放模式（无 NORVELS_AUTH_TOKEN）：一切照旧，个人数据归默认账号；
//   - 配置了 token：Bearer 仍可访问全部 API（管理侧、部署脚本）；
//     新会话 Cookie 同样放行（普通读者）；两者并存不互斥。

// TestLegacyBearerStillWorksWithAccounts 配置口令后 Bearer 照常可用。
func TestLegacyBearerStillWorksWithAccounts(t *testing.T) {
	e := authEnv(t, "test123")
	if rr := e.do(http.MethodGet, "/api/shelf/ids", nil, map[string]string{
		"Authorization": "Bearer test123",
	}); rr.Code != http.StatusOK {
		t.Fatalf("Bearer 访问书架 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
}

// TestSessionCookieWorksWhenTokenConfigured 口令开启时，会话 Cookie 也能进门。
func TestSessionCookieWorksWhenTokenConfigured(t *testing.T) {
	e := authEnv(t, "test123")
	// 先过口令建号：注册端点要求先持凭证。
	reg := e.do(http.MethodPost, "/api/auth/register", jsonBody(nameJSON("读者甲")), map[string]string{
		"Authorization": "Bearer test123",
		"Content-Type":  "application/json",
	})
	if reg.Code != http.StatusCreated {
		t.Fatalf("带口令注册 = %d，期望 201，body=%s", reg.Code, reg.Body.String())
	}
	token := findCookie(t, reg, SessionCookieName).Value
	if rr := e.do(http.MethodGet, "/api/shelf/ids", nil, withSession(token)); rr.Code != http.StatusOK {
		t.Errorf("会话 Cookie 访问书架 = %d，期望 200", rr.Code)
	}
	// 不带任何凭证仍然 401。
	if rr := e.do(http.MethodGet, "/api/shelf/ids", nil, nil); rr.Code != http.StatusUnauthorized {
		t.Errorf("无凭证 = %d，期望 401", rr.Code)
	}
}

// TestRegisterRequiresTokenWhenConfigured 口令开启时不给未持凭证者自助建号的口子。
func TestRegisterRequiresTokenWhenConfigured(t *testing.T) {
	e := authEnv(t, "test123")
	rr := e.do(http.MethodPost, "/api/auth/register", jsonBody(nameJSON("路人")), map[string]string{
		"Content-Type": "application/json",
	})
	requireError(t, rr, http.StatusUnauthorized, codeUnauthorized)
}

// TestOpenModeUsesDefaultUser 开放模式没有登录这回事，个人数据归默认账号。
func TestOpenModeUsesDefaultUser(t *testing.T) {
	e := newEnv(t)
	rr := e.do(http.MethodGet, "/api/auth/me", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("开放模式 /api/auth/me = %d，body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeBody[authUser](t, rr); got.Name != "reader" {
		t.Errorf("开放模式当前账号 = %q，期望 reader", got.Name)
	}
}
