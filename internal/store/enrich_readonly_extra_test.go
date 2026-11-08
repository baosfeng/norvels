package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenReadOnlyMissingContentDB 覆盖正文库不存在：OpenReadOnly 报错而不是静默降级。
func TestOpenReadOnlyMissingContentDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "only-main.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MigrateTo(context.Background(), LatestVersion()); err != nil {
		st.Close()
		t.Fatal(err)
	}
	st.Close() //nolint:errcheck // 测试收尾
	// 删掉正文库：OpenReadOnly 必须报「打不开」，不能悄悄只用主库（正文读会全空）。
	if err := os.Remove(ContentDBPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(context.Background(), path); err == nil {
		t.Fatal("正文库缺失时只读打开应报错")
	}
}

// TestOpenReadOnlyEmptyPath 覆盖空路径：直接报错（不 panic、不去 mkdir）。
func TestOpenReadOnlyEmptyPath(t *testing.T) {
	if _, err := OpenReadOnly(context.Background(), ""); err == nil {
		t.Fatal("空路径应报错")
	}
}

// TestRetryOnLockedContextCancel 覆盖重试中途 ctx 取消：立即返回 ctx 错误。
func TestRetryOnLockedContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := RetryOnLocked(ctx, 5, func() error {
		calls++
		cancel()
		return errFakeLocked{}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx 取消应返回 context.Canceled，实际 %v", err)
	}
	if calls != 1 {
		t.Fatalf("取消后不该再重试，实际 %d 次", calls)
	}
}

// TestRetryOnLockedZeroAttempts 覆盖 attempts<=0：至少执行一次。
func TestRetryOnLockedZeroAttempts(t *testing.T) {
	calls := 0
	if err := RetryOnLocked(context.Background(), 0, func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("attempts<=0 也应执行一次，实际 %d", calls)
	}
}
