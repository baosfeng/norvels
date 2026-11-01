package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"norvels/internal/recstore"
)

// openTempRec 打开一个临时推荐库（清理相关用例共用）。
func openTempRec(t *testing.T) *recstore.Store {
	t.Helper()
	rec, err := recstore.Open(context.Background(), filepath.Join(t.TempDir(), "norvels-rec.db"))
	if err != nil {
		t.Fatalf("recstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })
	return rec
}

func TestPruneOnceRemovesExpiredEvents(t *testing.T) {
	rec := openTempRec(t)
	ctx := context.Background()
	old := time.Now().UTC().AddDate(0, 0, -200).Format("2006-01-02T15:04:05.000Z")
	if _, err := rec.RecordEvents(ctx, 1, []recstore.Event{
		{Kind: recstore.KindClick, BookID: 1, TS: old},
	}); err != nil {
		t.Fatalf("写入旧事件: %v", err)
	}

	logs := captureLog(t)
	pruneOnce(ctx, rec)
	if !strings.Contains(logs(), "已清理 1 条") {
		t.Fatalf("应记录清理条数，实际日志: %q", logs())
	}
}

func TestPruneOnceLogsFailure(t *testing.T) {
	rec := openTempRec(t)
	_ = rec.Close()
	logs := captureLog(t)
	pruneOnce(context.Background(), rec) // 库已关闭 → 只记 WARN，不 panic
	if !strings.Contains(logs(), "清理失败") {
		t.Fatalf("失败应记 WARN，实际日志: %q", logs())
	}
}

func TestPruneRecStoreStopsOnCancel(t *testing.T) {
	rec := openTempRec(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pruneRecStore(ctx, rec)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("清理循环未随 ctx 取消退出")
	}
}

func TestPruneRecStoreWithNilStore(t *testing.T) {
	done := make(chan struct{})
	go func() {
		pruneRecStore(context.Background(), nil) // 推荐库没打开：直接返回，不阻塞启动
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rec 为 nil 时应立即返回")
	}
}
