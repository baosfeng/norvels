package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestTagRulesRoundTrip 词表规则存取往返；未配置返回空切片。
func TestTagRulesRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	requireTagRulesLen(t, ctx, st, 0)
	saved := []TagRule{
		{Tag: "奇幻", Keywords: []string{"魔法", "精灵"}},
		{Tag: "科幻", Keywords: []string{"飞船"}},
	}
	if err := st.SaveTagRules(ctx, saved); err != nil {
		t.Fatalf("SaveTagRules: %v", err)
	}
	requireTagRulesRoundTrip(t, ctx, st)
	if err := st.SaveTagRules(ctx, nil); err != nil {
		t.Fatalf("清空 SaveTagRules: %v", err)
	}
	requireTagRulesLen(t, ctx, st, 0)
}

// requireTagRulesLen 断言规则条数（空切片）。
func requireTagRulesLen(t *testing.T, ctx context.Context, st *Store, want int) {
	t.Helper()
	rules, err := st.ListTagRules(ctx)
	if err != nil || len(rules) != want {
		t.Fatalf("ListTagRules = %+v, err=%v，期望 %d 条", rules, err, want)
	}
}

// requireTagRulesRoundTrip 断言两条规则的字段往返。
func requireTagRulesRoundTrip(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	got, err := st.ListTagRules(ctx)
	if err != nil || len(got) != 2 || got[0].Tag != "奇幻" || len(got[1].Keywords) != 1 {
		t.Fatalf("ListTagRules = %+v, err=%v", got, err)
	}
}

// TestTagRulesCorruptJSON 坏配置报错而不是静默清空（否则用户规则被无声删掉）。
func TestTagRulesCorruptJSON(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.PutSetting(ctx, SettingKeyTagRules, "{not-json"); err != nil {
		t.Fatalf("PutSetting: %v", err)
	}
	if _, err := st.ListTagRules(ctx); err == nil {
		t.Error("坏 JSON 应当报错")
	}
}

// TestImportTagsOnlyAdd 导入打标只增不删：手动标签不被覆盖，同名不重复插。
func TestImportTagsOnlyAdd(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := sampleBook("打标之书", "打标作者", "/books/tagging.txt", strings.Repeat("8", 64))
	b.Subjects = []string{"EPUB分类"}
	b.AutoTags = []string{"词表标签"}
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	requireTagSource(t, ctx, st, res.BookID, "EPUB分类", TagSourceEPUB)
	requireTagSource(t, ctx, st, res.BookID, "词表标签", TagSourceAuto)

	// 用户手动改成只剩「手动标签」：全量替换是显式操作，允许清掉自动标签。
	if err := st.SetBookTags(ctx, res.BookID, []string{"手动标签"}); err != nil {
		t.Fatalf("SetBookTags: %v", err)
	}
	// 再覆盖导入（带同样的自动标签）：只增，不动手动的「手动标签」。
	if _, err := st.SaveBook(ctx, b, SaveOptions{Force: true}); err != nil {
		t.Fatalf("覆盖导入: %v", err)
	}
	tags, err := st.ListBookTags(ctx, res.BookID)
	if err != nil {
		t.Fatalf("ListBookTags: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("覆盖导入后标签 = %v，期望「手动标签」保留且自动标签补回（3 个）", tags)
	}
	requireTagSource(t, ctx, st, res.BookID, "手动标签", TagSourceManual)
	requireTagSource(t, ctx, st, res.BookID, "词表标签", TagSourceAuto)
}

// requireTagSource 断言某书某标签的来源。
func requireTagSource(t *testing.T, ctx context.Context, st *Store, bookID int64, tag, want string) {
	t.Helper()
	var source string
	err := st.DB().QueryRowContext(ctx,
		"SELECT bt.source FROM book_tags bt JOIN tags t ON t.id = bt.tag_id "+
			"WHERE bt.book_id = ? AND t.name = ?", bookID, tag).Scan(&source)
	if err != nil {
		t.Fatalf("读标签 %q 来源: %v", tag, err)
	}
	if source != want {
		t.Errorf("标签 %q 来源 = %q，期望 %q", tag, source, want)
	}
}

// TestBookExistsForImport 复刻 SaveBook 的匹配顺序（SHA/源路径 + 追更时的书名作者兜底）。
func TestBookExistsForImport(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := sampleBook("查重之书", "查重作者", "/books/dedup.txt", strings.Repeat("9", 64))
	if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	cases := []struct {
		name        string
		source, sha string
		title, auth string
		update      bool
		want        bool
	}{
		{"同源路径", "/books/dedup.txt", "zz", "", "", false, true},
		{"同 SHA", "/books/other.txt", strings.Repeat("9", 64), "", "", false, true},
		{"追更指纹兜底", "/books/new-path.txt", "zz", "查重之书", "查重作者", true, true},
		{"非追更不看指纹", "/books/new-path.txt", "zz", "查重之书", "查重作者", false, false},
		{"全新", "/books/fresh.txt", "zz", "别的书", "别人", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := st.BookExistsForImport(ctx, tc.source, tc.sha, tc.title, tc.auth, tc.update)
			if err != nil || got != tc.want {
				t.Errorf("BookExistsForImport = %v, err=%v，期望 %v", got, err, tc.want)
			}
		})
	}
}

var _ = model.ParsedChapter{}
