package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件补正文库的库级行为（路径推导 / 双库 Ping / 对账在未迁移库上的表现），
// 这些是「拆库」之后运维与启动路径都会碰到的边界。

// TestContentDBPathDerivation 正文库路径推导：兄弟文件 + 环境变量覆盖。
func TestContentDBPathDerivation(t *testing.T) {
	t.Setenv(EnvContentDB, "")
	cases := []struct{ main, want string }{
		{"/data/norvels.db", "/data/norvels-content.db"},
		{"/data/library.db", "/data/library-content.db"},
		{"/data/novel", "/data/novel-content"},
		{"norvels.db", "norvels-content.db"},
	}
	for _, c := range cases {
		if got := ContentDBPath(c.main); got != c.want {
			t.Errorf("ContentDBPath(%q) = %q，期望 %q", c.main, got, c.want)
		}
	}
	t.Setenv(EnvContentDB, "/tmp/custom-content.db")
	if got := ContentDBPath("/data/norvels.db"); got != "/tmp/custom-content.db" {
		t.Errorf("环境变量未生效：%q", got)
	}
}

// TestStorePingAndCheckpoint 两个库都要健康：Ping 任一失败即返回错误。
func TestStorePingAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := st.Checkpoint(ctx); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	// 关掉之后再 Ping 必须报错（证明它确实在 ping 两个库，而不是只看主库）。
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := st.Ping(ctx); err == nil {
		t.Error("Close 之后 Ping 应该失败")
	}
}

// TestReconcileContentMissingTable 未迁移到 0018 的库：报「缺表」而不是谎报干净。
func TestReconcileContentMissingTable(t *testing.T) {
	ctx := context.Background()
	st, _ := openBare(t, "bare-reconcile.db")
	rep, err := st.ReconcileContent(ctx, false)
	if err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	if !rep.MissingTable {
		t.Errorf("未迁移的库应标记 MissingTable：%+v", rep)
	}
	if rep.OrphanBlocks != 0 || rep.DanglingRefs != 0 {
		t.Errorf("缺表时不该报数：%+v", rep)
	}
}

// TestContentReconcileReportString 对账结果的日志文案（运维排障的第一手证据）。
func TestContentReconcileReportString(t *testing.T) {
	rep := ContentReconcileReport{OrphanBlocks: 7, OrphanBooks: 2, DanglingRefs: 1, RemovedBlocks: 7}
	out := rep.String()
	for _, want := range []string{"孤儿块 7", "2 本书", "悬空引用 1", "已删除 7"} {
		if !strings.Contains(out, want) {
			t.Errorf("文案缺少 %q：%s", want, out)
		}
	}
	missing := ContentReconcileReport{MissingTable: true}.String()
	if !strings.Contains(missing, "缺 chapter_blocks") {
		t.Errorf("缺表文案不明确：%s", missing)
	}
}

// TestHealthReportReconcileSnapshot 对账结果来自后台快照（对账要全表扫，不能每次请求都跑）。
//
// 语义在 0023 改了：请求**不再**同步跑第一次对账，而是立刻返回 stale 快照，
// 由后台 goroutine 刷新（见 health_refresh_test.go）。这里锁死请求侧的那一半：
// 同一份快照被连续两次请求复用，且第一次不等对账返回完整 200。
func TestHealthReportReconcileSnapshot(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first := st.HealthReport(ctx).Content
	if first == nil {
		t.Fatal("首次健康报告缺少 content")
	}
	if !first.Stale || first.CheckedAt != "" {
		t.Errorf("尚无快照时应 stale 且不带 CheckedAt：%+v", first)
	}
	// 跑一次对账后快照落库，后续请求复用同一份（不重跑）。
	if _, err := st.ReconcileContent(ctx, false); err != nil {
		t.Fatalf("ReconcileContent: %v", err)
	}
	seeded := st.HealthReport(ctx).Content
	if seeded == nil || seeded.Stale || seeded.CheckedAt == "" {
		t.Fatalf("对账后应给出新鲜快照：%+v", seeded)
	}
	second := st.HealthReport(ctx).Content
	if second == nil || second.CheckedAt != seeded.CheckedAt {
		t.Errorf("第二次报告没有复用快照：%+v vs %+v", second, seeded)
	}
}

// TestContentDBSiblingFileCreated 打开即建好正文库文件（0018 之前也不会缺文件）。
func TestContentDBSiblingFileCreated(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close() //nolint:errcheck // 测试收尾
	if !strings.HasSuffix(st.ContentPath(), "app-content.db") {
		t.Errorf("正文库路径 = %q", st.ContentPath())
	}
	if err := st.Ping(ctx); err != nil {
		t.Errorf("正文库文件应已可 ping：%v", err)
	}
}
