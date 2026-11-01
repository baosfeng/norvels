package api

import (
	"context"
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
