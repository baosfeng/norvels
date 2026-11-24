package observe

import (
	"database/sql"
	"testing"
	"time"
)

// noteActivity 打点后 IdleFor 必须反映「刚刚有过活动」，否则闲置归还循环
// 会在用户正在读的时候把页缓存抽走。
func TestIdleForTracksActivity(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	if got := IdleFor(); got < time.Hour {
		t.Fatalf("从未活动时应视为久闲，实际 %v", got)
	}
	if !LastActivity().IsZero() {
		t.Fatal("从未活动时 LastActivity 应为零值")
	}
	noteActivity()
	if got := IdleFor(); got > time.Second {
		t.Fatalf("刚打点后应视为活跃，实际闲置 %v", got)
	}
	if LastActivity().IsZero() {
		t.Fatal("打点后 LastActivity 不应为零值")
	}
}

// SQL 活动必须经由驱动包装层打点：这条走真实 Query，验证的是接线而不是内部函数。
func TestQueryNotesActivity(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck // 测试结束关闭
	if IdleFor() < time.Hour {
		t.Fatal("建库后尚未 Query，不应算活跃")
	}
	var n int
	if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got := IdleFor(); got > time.Second {
		t.Fatalf("Query 之后应算活跃，实际闲置 %v", got)
	}
}
