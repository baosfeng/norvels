package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"norvels/internal/store"
)

// 本文件锁死「去重阈值」在 api 侧的边界：settings 读一次、注入导入选项、坏配置回落缺省。
// 判定逻辑本身在 store / importer 侧（各自的测试里），这里只验跨包传参不丢。

// TestLoadDedupeThreshold 缺省、合法、非法三种配置的取值。
func TestLoadDedupeThreshold(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	if got := env.srv.loadDedupeThreshold(ctx); got != store.DefaultDedupeThreshold {
		t.Errorf("预置阈值 = %v，期望 %v", got, store.DefaultDedupeThreshold)
	}
	if err := env.st.PutSetting(ctx, store.SettingKeyDedupeThreshold, "0.5"); err != nil {
		t.Fatalf("写阈值: %v", err)
	}
	if got := env.srv.loadDedupeThreshold(ctx); got != 0.5 {
		t.Errorf("阈值 = %v，期望 0.5", got)
	}
	// 非法值（百分数写法）不拦导入：回落缺省，只记日志。
	if err := env.st.PutSetting(ctx, store.SettingKeyDedupeThreshold, "95"); err != nil {
		t.Fatalf("写非法阈值: %v", err)
	}
	if got := env.srv.loadDedupeThreshold(ctx); got != store.DefaultDedupeThreshold {
		t.Errorf("非法阈值应回落 %v，实际 %v", store.DefaultDedupeThreshold, got)
	}
}

// TestImportOptionsCarryDedupeThreshold 目录导入与上传导入都必须带上阈值（漏一条就等于阈值不生效）。
func TestImportOptionsCarryDedupeThreshold(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	if err := env.st.PutSetting(ctx, store.SettingKeyDedupeThreshold, "0.8"); err != nil {
		t.Fatalf("写阈值: %v", err)
	}

	dir := t.TempDir()
	opts := env.srv.importOptions(ctx, importRequest{Dir: dir}, dir, 0, nil, nil, nil)
	if opts.DedupeThreshold != 0.8 {
		t.Errorf("目录导入阈值 = %v，期望 0.8", opts.DedupeThreshold)
	}
	sess := &uploadSession{dir: dir}
	if got := env.srv.uploadOptions(ctx, sess, 0, nil, nil).DedupeThreshold; got != 0.8 {
		t.Errorf("上传导入阈值 = %v，期望 0.8", got)
	}
}

// TestDedupeSettingsEndpoint 读改写闭环：缺省 0.95 → PUT 0.8 → GET 0.8（写库即生效）。
func TestDedupeSettingsEndpoint(t *testing.T) {
	env := newEnv(t)
	rr := env.get("/api/settings/dedupe")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET 状态码 = %d，期望 200（body=%s）", rr.Code, rr.Body.String())
	}
	if body := decodeBody[dedupeSettingsBody](t, rr); body.Threshold != store.DefaultDedupeThreshold || body.Warning != "" {
		t.Fatalf("GET = %+v，期望缺省阈值且无 warning", body)
	}

	rr = env.do(http.MethodPut, "/api/settings/dedupe", strings.NewReader("{\"threshold\":0.8}"),
		map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("PUT 状态码 = %d，期望 204（body=%s）", rr.Code, rr.Body.String())
	}
	if body := decodeBody[dedupeSettingsBody](t, env.get("/api/settings/dedupe")); body.Threshold != 0.8 {
		t.Errorf("PUT 后 GET = %+v，期望 0.8", body)
	}
	// 落库的值与导入链路读到的是同一个：改完立刻影响下一次导入。
	if got, err := env.st.DedupeThreshold(context.Background()); err != nil || got != 0.8 {
		t.Errorf("store 侧阈值 = %v（err=%v），期望 0.8", got, err)
	}
}

// TestDedupeSettingsRejectsInvalid PUT 越界与怪值一律 400（不静默夹紧），
// 而库里已有的坏值在 GET 时按生效值 + warning 返回，不让驾驶舱 500。
func TestDedupeSettingsRejectsInvalid(t *testing.T) {
	env := newEnv(t)
	for _, body := range []string{
		"{\"threshold\":0}",
		"{\"threshold\":-0.5}",
		"{\"threshold\":95}",
		"{\"threshold\":1.5}",
		"{\"threshold\":\"high\"}",
		"{}",
	} {
		rr := env.do(http.MethodPut, "/api/settings/dedupe", strings.NewReader(body),
			map[string]string{"Content-Type": "application/json"})
		requireError(t, rr, http.StatusBadRequest, codeInvalidParam)
	}
	if err := env.st.PutSetting(context.Background(), store.SettingKeyDedupeThreshold, "95"); err != nil {
		t.Fatalf("写坏值: %v", err)
	}
	got := decodeBody[dedupeSettingsBody](t, env.get("/api/settings/dedupe"))
	if got.Threshold != store.DefaultDedupeThreshold || got.Warning == "" {
		t.Errorf("坏配置下 GET = %+v，期望回落缺省并带 warning", got)
	}
}
