package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestOpenReadOnlyRefusesWrite 覆盖只读打开：能读、写入必须失败。
func TestOpenReadOnlyRefusesWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MigrateTo(context.Background(), LatestVersion()); err != nil {
		st.Close()
		t.Fatal(err)
	}
	seedEnrichBook(t, st)
	st.Close() //nolint:errcheck // 测试收尾

	ro, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatalf("只读打开失败: %v", err)
	}
	defer ro.Close() //nolint:errcheck // 测试收尾
	list, err := ro.EnrichCandidates(context.Background(), EnrichKindMissingIntro, 5)
	if err != nil {
		t.Fatalf("只读连接应能查: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("只读连接应查得到种子书")
	}
	if _, err := ro.DB().ExecContext(context.Background(), "DELETE FROM books"); err == nil {
		t.Fatal("只读连接不该允许写")
	}
}

// TestRetryOnLocked 覆盖重试：前两次失败、第三次成功；非锁错误立即返回。
func TestRetryOnLocked(t *testing.T) {
	ctx := context.Background()
	calls := 0
	err := RetryOnLocked(ctx, 4, func() error {
		calls++
		if calls < 3 {
			return errFakeLocked{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("第三次应成功，实际 %v", err)
	}
	if calls != 3 {
		t.Fatalf("应重试到第三次，实际调用 %d 次", calls)
	}
	calls = 0
	plain := RetryOnLocked(ctx, 4, func() error {
		calls++
		return errPlain{}
	})
	if plain == nil || calls != 1 {
		t.Fatalf("非锁错误应只调用一次，实际 %d 次 / %v", calls, plain)
	}
}

// errFakeLocked 是「database is locked」形态的错误。
type errFakeLocked struct{}

func (errFakeLocked) Error() string { return "SQL logic error: database is locked (5)" }

// errPlain 是不可重试的错误。
type errPlain struct{}

func (errPlain) Error() string { return "no such table: books" }
