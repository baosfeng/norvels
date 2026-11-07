package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"norvels/internal/store"
)

// 驾驶舱聚合的「不完整结果」契约：
//   - 块为空却与 stats 矛盾（书数 > 0 但健康卡说 0 本、作者榜空、读过书却没有最近在读）
//     → 写入 warnings 说明「尚未就绪」，让调用方能分辨「真的没有」与「没取到」；
//   - 带 warning 的结果**不进 TTL 缓存**（下一次请求重新真算），完整结果才进缓存。
//
// 为什么必须这样：七块是并发取的，某块「成功返回空结果」与「库里真的没有」
// 在响应里长得一模一样；这样的结果一旦被缓存，驾驶舱会整页空态挂满一个 TTL。

const (
	warnHealth   = "health 尚未就绪"
	warnAuthors  = "topAuthors 尚未就绪"
	warnProgress = "readingProgress 尚未就绪"
)

// TestDashboardInconsistencies 一致性校验：矛盾才告警，空库 / 没读过不告警。
func TestDashboardInconsistencies(t *testing.T) {
	zero := 0
	books := 5
	full := &dashboardPayload{
		Stats:           &store.Stats{Books: 5, Authors: 3},
		Health:          &store.HealthReport{TotalBooks: &books},
		TopAuthors:      &store.Page[store.AuthorSummary]{Items: []store.AuthorSummary{{ID: 1}}},
		ReadingStats:    &store.ReadingStats{TotalSeconds: 120},
		ReadingProgress: []store.ReadingProgress{{BookID: 1}},
	}
	if got := dashboardInconsistencies(full); len(got) != 0 {
		t.Fatalf("完整结果不该有告警: %v", got)
	}

	cases := []struct {
		name    string
		payload *dashboardPayload
		want    string
	}{
		{"健康块整块缺席", &dashboardPayload{Stats: &store.Stats{Books: 5}}, warnHealth},
		{"健康块说 0 本", &dashboardPayload{Stats: &store.Stats{Books: 5},
			Health: &store.HealthReport{TotalBooks: &zero}}, "health.totalBooks 尚未就绪"},
		{"健康块书数这次失败", &dashboardPayload{Stats: &store.Stats{Books: 5},
			Health: &store.HealthReport{}}, "health.totalBooks 尚未就绪"},
		{"作者榜空但有作者", &dashboardPayload{Stats: &store.Stats{Books: 5, Authors: 3},
			Health:     &store.HealthReport{TotalBooks: &books},
			TopAuthors: &store.Page[store.AuthorSummary]{}}, warnAuthors},
		{"最近在读空但读过书", &dashboardPayload{Stats: &store.Stats{Books: 5, Authors: 3},
			Health:       &store.HealthReport{TotalBooks: &books},
			TopAuthors:   &store.Page[store.AuthorSummary]{Items: []store.AuthorSummary{{ID: 1}}},
			ReadingStats: &store.ReadingStats{TotalSeconds: 120}}, warnProgress},
	}
	for _, c := range cases {
		got := strings.Join(dashboardInconsistencies(c.payload), " | ")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s：warnings = %q，期望含 %q", c.name, got, c.want)
		}
	}

	// 合法空态：空库、没读过书的账号——不告警（否则每请求都带 warning 且永不进缓存）。
	empty := &dashboardPayload{
		Stats:           &store.Stats{Books: 0, Authors: 0},
		Health:          &store.HealthReport{TotalBooks: &zero},
		TopAuthors:      &store.Page[store.AuthorSummary]{},
		ReadingStats:    &store.ReadingStats{},
		ReadingProgress: []store.ReadingProgress{},
	}
	if got := dashboardInconsistencies(empty); len(got) != 0 {
		t.Fatalf("空库 / 没读过不该告警: %v", got)
	}
}

// TestDashboardCacheSkipsIncomplete 不完整结果不缓存：连打两次第二次仍真算；完整结果才缓存。
func TestDashboardCacheSkipsIncomplete(t *testing.T) {
	c := newDashboardCache(time.Minute)
	ctx := context.Background()
	calls := 0
	incomplete := dashboardPayload{Warnings: []string{warnHealth + "（可能是启动回填 / 瞬时空结果）"}}
	loadIncomplete := func(context.Context) (dashboardPayload, error) {
		calls++
		return incomplete, nil
	}
	for i := 1; i <= 2; i++ {
		got, cached, err := c.do(ctx, "k", loadIncomplete)
		if err != nil {
			t.Fatalf("第 %d 次 do: %v", i, err)
		}
		if cached {
			t.Fatalf("第 %d 次不该命中缓存（带 warning 的结果不进缓存）", i)
		}
		if len(got.Warnings) == 0 {
			t.Fatalf("第 %d 次丢了 warning", i)
		}
	}
	if calls != 2 {
		t.Fatalf("加载次数 = %d，期望 2（不完整结果每次都真算）", calls)
	}

	// 换成完整结果：这一次真算，下一次就命中缓存。
	complete := dashboardPayload{Stats: &store.Stats{Books: 5}}
	loadComplete := func(context.Context) (dashboardPayload, error) {
		calls++
		return complete, nil
	}
	if _, cached, _ := c.do(ctx, "k", loadComplete); cached {
		t.Fatal("完整结果的首次计算不该是 cached")
	}
	if _, cached, _ := c.do(ctx, "k", loadComplete); !cached {
		t.Fatal("完整结果在 TTL 内应命中缓存")
	}
	if calls != 3 {
		t.Fatalf("加载次数 = %d，期望 3（完整结果只算一次）", calls)
	}
}

// TestDashboardIncompleteWarnsAndIsNotCached 端到端：块与 stats 矛盾时带 warning，且连续两次都真算。
//
// 现场构造：authors / books 都有数据，但 0022 的物化表 author_agg 没有对应行
// （= 启动回填未完成 / 物化表被重建）；作者榜会「成功返回空」，正是要拦住的那类结果。
func TestDashboardIncompleteWarnsAndIsNotCached(t *testing.T) {
	e := newEnv(t)
	if _, err := e.st.DB().Exec(
		"INSERT INTO authors(id, name) VALUES (1, '示例作者')"); err != nil {
		t.Fatalf("插作者: %v", err)
	}
	if _, err := e.st.DB().Exec(
		"INSERT INTO books(title, author_id, source_path, file_sha256, word_count, chapter_count) " +
			"VALUES ('示例书', 1, '/path/to/txt/a.txt', 'aaaa', 1000, 3)"); err != nil {
		t.Fatalf("插书: %v", err)
	}
	for i := 1; i <= 2; i++ {
		rr := e.get("/api/dashboard")
		if rr.Code != 200 {
			t.Fatalf("第 %d 次状态码 = %d，body=%s", i, rr.Code, rr.Body.String())
		}
		d := decodeBody[dashboardPayload](t, rr)
		if !strings.Contains(strings.Join(d.Warnings, " | "), warnAuthors) {
			t.Fatalf("第 %d 次 warnings = %v，期望含 %q", i, d.Warnings, warnAuthors)
		}
		if d.Cached {
			t.Fatalf("第 %d 次命中了缓存：带 warning 的聚合不该进缓存", i)
		}
		if d.Stats == nil || d.Stats.Books != 1 || d.Stats.Authors != 1 {
			t.Fatalf("stats = %+v，期望 books=1 authors=1（矛盾点就靠它）", d.Stats)
		}
	}
}

// TestDashboardCompleteIsCached 对照：自洽的完整结果两次请求，第二次命中缓存。
func TestDashboardCompleteIsCached(t *testing.T) {
	e := newEnvWith(t, Config{DisableAccessLog: true, DashboardTTL: time.Minute}, testFS())
	first := decodeBody[dashboardPayload](t, e.get("/api/dashboard"))
	if len(first.Warnings) != 0 {
		t.Fatalf("空库聚合应当自洽: %v", first.Warnings)
	}
	if first.Cached {
		t.Fatal("首次不该命中缓存")
	}
	if second := decodeBody[dashboardPayload](t, e.get("/api/dashboard")); !second.Cached {
		t.Fatal("完整结果第二次应命中缓存")
	}
}
