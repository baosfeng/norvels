package api

import (
	"net/http"
	"testing"
)

// 发版说明端点（GET /api/releases）：只读、无参数，两个失败口径都要有覆盖——
// 解析失败必须是 500（不能拿空数组顶替），版本对不上时 current 留空。

func TestReleasesEndpointReturnsBuiltin(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/releases")
	if rr.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，body=%s", rr.Code, rr.Body.String())
	}
	got := decodeBody[releasesResponse](t, rr)
	if len(got.Items) < 17 {
		t.Fatalf("条数 = %d，期望至少 17 个历史版本", len(got.Items))
	}
	// 倒序：第一条的版本号不小于最后一条。
	first, last := got.Items[0], got.Items[len(got.Items)-1]
	if !versionAtLeast(first.Version, last.Version) {
		t.Fatalf("未按版本倒序：首条 %s、末条 %s", first.Version, last.Version)
	}
	for _, e := range got.Items {
		if e.Version == "" || e.Sections.Added == nil {
			t.Fatalf("条目字段不全: %+v", e)
		}
	}
}

func TestReleasesEndpointMarksCurrent(t *testing.T) {
	// 造一个「当前版本确实在说明里」的环境：直接用内置说明里的版本号注入。
	env := newEnv(t)
	current := firstVersionInBuiltin(t)
	env.srv.cfg.Version = current
	got := decodeBody[releasesResponse](t, env.get("/api/releases"))
	if got.Current != current {
		t.Fatalf("current = %q，期望 %q", got.Current, current)
	}
}

func TestReleasesEndpointDevBuildHasNoCurrent(t *testing.T) {
	env := newEnvWith(t, Config{Version: "", DisableAccessLog: true}, testFS())
	got := decodeBody[releasesResponse](t, env.get("/api/releases"))
	if got.Current != "" {
		t.Fatalf("dev 构建 current = %q，期望空", got.Current)
	}
}

func TestReleasesEndpointBrokenChangelogIs500(t *testing.T) {
	env := newEnv(t)
	env.srv.releaseMD = []byte("## [0.1.0] - 2026-01-01\n### 未知分组\n- 甲\n")
	rr := env.get("/api/releases")
	requireError(t, rr, http.StatusInternalServerError, codeInternal)
	// 错误响应里不能漏出内部细节（说明文件路径 / 行号）。
	if body := rr.Body.String(); containsAnyString(body, "CHANGELOG") {
		t.Fatalf("错误响应泄露了内部文件名: %s", body)
	}
}

func TestReleasesMethodNotAllowed(t *testing.T) {
	env := newEnv(t)
	rr := env.do(http.MethodPost, "/api/releases", nil, nil)
	requireError(t, rr, http.StatusMethodNotAllowed, codeMethodNotAllowed)
}

// firstVersionInBuiltin 取内置说明里最新版本号，用来构造「版本对得上」的环境。
func firstVersionInBuiltin(t *testing.T) string {
	t.Helper()
	notes := decodeBody[releasesResponse](t, newEnv(t).get("/api/releases"))
	return notes.Items[0].Version
}

// versionAtLeast 比较两个 x.y.z 版本号（a >= b）。
func versionAtLeast(a, b string) bool {
	pa, pb := semverParts(a), semverParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return true
}

// semverParts / compare 是测试用的最小版本号比较，不复用被断言方（internal/releases）的实现，
// 免得「排序错了但断言跟着一起错」。
func semverParts(v string) [3]int {
	var out [3]int
	idx := 0
	n := 0
	for i := 0; i < len(v); i++ {
		if v[i] >= '0' && v[i] <= '9' {
			n = n*10 + int(v[i]-'0')
			continue
		}
		if idx < 3 {
			out[idx] = n
		}
		idx++
		n = 0
	}
	if idx < 3 {
		out[idx] = n
	}
	return out
}

func containsAnyString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
