package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 本文件补的是「删掉字级分词」后失去间接覆盖的纯函数，以及几个此前未被单测
// 触达的小接口（Path/Ping/Label/BookExistsBySHA256）。它们都是简单函数，
// 但覆盖率防退化机制（scripts/gate-cover.sh）会盯着，不能被顺手删测试带下去。

// TestIsCJK 覆盖 CJK 区间的边界与非 CJK 字符。
func TestIsCJK(t *testing.T) {
	in := []rune{'长', '夜', 'あ', '한', 'ㄅ', '\U00020000'}
	for _, r := range in {
		if r == 'ㄅ' {
			continue // 注音符号不在计字区间内
		}
		if !isCJK(r) {
			t.Errorf("isCJK(%q) = false，期望 true", r)
		}
	}
	for _, r := range []rune{'a', 'Z', '1', ' ', '，', 'ㄅ'} {
		if isCJK(r) {
			t.Errorf("isCJK(%q) = true，期望 false", r)
		}
	}
}

// TestCountWords 覆盖「CJK 逐字计数 + 连续非 CJK 词计 1」的口径。
func TestCountWords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"空串", "", 0},
		{"全汉字", "长夜书简", 4},
		{"汉字与标点", "长夜，书简。", 4},
		{"纯英文按词", "the quick brown fox", 4},
		{"英文与标点", "hello, world!", 2},
		{"中英混排", "青川 sword 剑砚", 5},
		{"只有空白", "   \t\n  ", 0},
		{"只有标点", "，。！？", 0},
		{"数字", "123 456", 2},
		{"换行分隔的长段", "长夜书简\n青川剑砚\n", 8},
		{"扩展 B 逐字", "\U00020000\U00020001 长", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countWords(c.in); got != c.want {
				t.Errorf("countWords(%q) = %d，期望 %d", c.in, got, c.want)
			}
		})
	}
}

// TestBookExistsBySHA256 覆盖空哈希短路、命中与未命中。
func TestBookExistsBySHA256(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	sha := strings.Repeat("a", 64)
	if _, err := st.SaveBook(ctx, sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", sha), SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"命中", sha, true},
		{"大写哈希也命中", strings.ToUpper(sha), true},
		{"带空白也命中", "  " + sha + "  ", true},
		{"未命中", strings.Repeat("b", 64), false},
		{"空哈希短路为 false", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := st.BookExistsBySHA256(ctx, c.in)
			if err != nil {
				t.Fatalf("BookExistsBySHA256: %v", err)
			}
			if got != c.want {
				t.Errorf("BookExistsBySHA256(%q) = %v，期望 %v", c.in, got, c.want)
			}
		})
	}
}

// TestStorePathAndPing 覆盖连接探活与路径读取（健康检查与日志都要用）。
func TestStorePathAndPing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ping.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if got := st.Path(); got != path {
		t.Errorf("Path() = %q，期望 %q", got, path)
	}
	if err := st.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// TestMigrationLabelFormat 覆盖 CLI 日志里的迁移标签格式（「0006 0006_drop_fts.sql」）。
func TestMigrationLabelFormat(t *testing.T) {
	m := migration{Version: 6, Name: "0006_drop_fts.sql"}
	if got, want := m.Label(), "0006 0006_drop_fts.sql"; got != want {
		t.Errorf("Label() = %q，期望 %q", got, want)
	}
}

// TestEnsureMetricsAsyncRecomputes 覆盖后台补算入口与完成回调。
//
// 缓存缺失时 EnsureMetricsAsync 应当在后台算好并把 Stale 转 false，
// 并调用一次 OnMetricsRefreshed 注册的回调。
func TestEnsureMetricsAsyncRecomputes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)
	if err := st.clearMetrics(ctx); err != nil {
		t.Fatalf("清空 metrics: %v", err)
	}

	done := make(chan error, 1)
	st.OnMetricsRefreshed(func(err error) { done <- err })
	st.EnsureMetricsAsync(ctx)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("后台补算失败: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("后台补算未在 10s 内完成")
	}

	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if snap.Stale {
		t.Errorf("补算完成后不应 stale: %+v", snap)
	}
	if snap.TextBytes <= 0 || snap.ChapterWordSum <= 0 {
		t.Errorf("补算结果异常: %+v", snap)
	}
}

// TestEnsureMetricsAsyncSkipsWhenFresh 缓存有效时不应重复补算（回调不会被调用）。
func TestEnsureMetricsAsyncSkipsWhenFresh(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	called := make(chan struct{}, 1)
	st.OnMetricsRefreshed(func(error) { called <- struct{}{} })
	st.EnsureMetricsAsync(ctx)

	select {
	case <-called:
		t.Error("缓存有效时不应触发补算")
	case <-time.After(150 * time.Millisecond):
		// 预期：安静地不做任何事
	}
}
