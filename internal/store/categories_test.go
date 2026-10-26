package store

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// newTestStoreTo 建一个停在指定迁移版本的库：迁移语义（加列/回填/重放）只能在这个状态下测。
func newTestStoreTo(t *testing.T, version int) *Store {
	t.Helper()
	ctx := context.Background()
	st, _ := openBare(t, "v"+strconv.Itoa(version)+".db")
	if _, err := st.MigrateTo(ctx, version); err != nil {
		t.Fatalf("迁移到版本 %d: %v", version, err)
	}
	return st
}

// 分类（0015）的口径：
//   - 存量书在加列那一刻统一回填成默认分类（缺省「未分类」，可用 NORVELS_LEGACY_CATEGORY 覆盖），
//     迁移重放不再覆盖后来导入的书；
//   - 导入按来源目录写入 (l1, l2)，空串表示未分类；
//   - 列表按大/小分类筛选，聚合按 (l1, l2) 分组。

func TestMigration0015BackfillsLegacyCategory(t *testing.T) {
	ctx := context.Background()
	st := newTestStoreTo(t, 14)
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('老书', '/old/legacy.txt', 'legacy-sha')"); err != nil {
		t.Fatalf("插入存量书: %v", err)
	}
	if _, err := st.MigrateTo(ctx, 15); err != nil {
		t.Fatalf("迁移到 0015: %v", err)
	}
	if got := scalarText(t, ctx, st, "SELECT category_l1 FROM books WHERE source_path = '/old/legacy.txt'"); got != unclassifiedCategory {
		t.Errorf("存量书分类 = %q，期望默认值 %q", got, unclassifiedCategory)
	}
}

// TestMigration0015UsesConfiguredLegacyCategory 校验环境变量覆盖默认回填分类。
func TestMigration0015UsesConfiguredLegacyCategory(t *testing.T) {
	t.Setenv("NORVELS_LEGACY_CATEGORY", "部署方分类")
	ctx := context.Background()
	st := newTestStoreTo(t, 14)
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256) VALUES ('老书', '/old/legacy2.txt', 'legacy-sha-2')"); err != nil {
		t.Fatalf("插入存量书: %v", err)
	}
	if _, err := st.MigrateTo(ctx, 15); err != nil {
		t.Fatalf("迁移到 0015: %v", err)
	}
	if got := scalarText(t, ctx, st, "SELECT category_l1 FROM books WHERE source_path = '/old/legacy2.txt'"); got != "部署方分类" {
		t.Errorf("存量书分类 = %q，期望环境变量指定的「部署方分类」", got)
	}
}

// TestMigration0015ReplayKeepsNewCategories 校验迁移重放（删版本号重跑）不会覆盖新书分类：
// 只有「首次加列」那一次才回填。
func TestMigration0015ReplayKeepsNewCategories(t *testing.T) {
	ctx := context.Background()
	st := newTestStoreTo(t, 14)
	if _, err := st.MigrateTo(ctx, 15); err != nil {
		t.Fatalf("迁移到 0015: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		"INSERT INTO books(title, source_path, file_sha256, category_l1, category_l2) "+
			"VALUES ('新书', '/new/a.txt', 'new-sha', '玄幻', '东方玄幻')"); err != nil {
		t.Fatalf("插入新书: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 15"); err != nil {
		t.Fatalf("删版本记录: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("重放迁移: %v", err)
	}
	if got := scalarText(t, ctx, st, "SELECT category_l1 FROM books WHERE source_path = '/new/a.txt'"); got != "玄幻" {
		t.Errorf("重放后新书分类 = %q，期望保持「玄幻」", got)
	}
}

func TestSaveBookPersistsCategory(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := sampleBook("分类之书", "作者甲", "/books/玄幻/东方玄幻/分类之书.txt", strings.Repeat("a", 64))
	b.CategoryL1, b.CategoryL2 = "玄幻", "东方玄幻"
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	requireOK(t, "SaveBook", err)

	got, err := st.GetBook(ctx, res.BookID)
	requireOK(t, "GetBook", err)
	requireEqual(t, "大分类", got.CategoryL1, "玄幻")
	requireEqual(t, "小分类", got.CategoryL2, "东方玄幻")
}

func TestBookFilterByCategory(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seed := func(title, path, l1, l2, sha string) {
		t.Helper()
		b := sampleBook(title, "作者甲", path, sha)
		b.CategoryL1, b.CategoryL2 = l1, l2
		_, err := st.SaveBook(ctx, b, SaveOptions{})
		requireOK(t, "SaveBook "+title, err)
	}
	seed("玄幻甲", "/books/玄幻/东方玄幻/甲.txt", "玄幻", "东方玄幻", strings.Repeat("b", 64))
	seed("玄幻乙", "/books/玄幻/奇幻玄幻/乙.txt", "玄幻", "奇幻玄幻", strings.Repeat("c", 64))
	seed("历史丙", "/books/历史/两宋元明/丙.txt", "历史", "两宋元明", strings.Repeat("d", 64))

	page, err := st.ListBooks(ctx, BookQuery{CategoryL1: "玄幻", Page: 1, Size: 10})
	requireOK(t, "按大分类筛选", err)
	requireEqual(t, "大分类命中的书数", page.Total, 2)

	page, err = st.ListBooks(ctx, BookQuery{CategoryL1: "玄幻", CategoryL2: "奇幻玄幻", Page: 1, Size: 10})
	requireOK(t, "按小分类筛选", err)
	requireEqual(t, "小分类命中的书数", page.Total, 1)
	requireEqual(t, "小分类命中的书名", page.Items[0].Title, "玄幻乙")

	page, err = st.ListBooks(ctx, BookQuery{Page: 1, Size: 10})
	requireOK(t, "无筛选", err)
	requireEqual(t, "无筛选书数", page.Total, 3)
}

func TestListCategoriesGroups(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seed := func(title, path, l1, l2, sha string) {
		t.Helper()
		b := sampleBook(title, "作者甲", path, sha)
		b.CategoryL1, b.CategoryL2 = l1, l2
		_, err := st.SaveBook(ctx, b, SaveOptions{})
		requireOK(t, "SaveBook "+title, err)
	}
	seed("甲", "/books/玄幻/东方玄幻/甲.txt", "玄幻", "东方玄幻", strings.Repeat("e", 64))
	seed("乙", "/books/玄幻/东方玄幻/乙.txt", "玄幻", "东方玄幻", strings.Repeat("f", 64))
	seed("丙", "/books/历史/两宋元明/丙.txt", "历史", "两宋元明", strings.Repeat("0", 64))

	rows, err := st.ListCategories(ctx)
	requireOK(t, "ListCategories", err)
	requireEqual(t, "分组行数", len(rows), 2)
	requireEqual(t, "首个分组的大分类", rows[0].L1, "历史")
	requireEqual(t, "首个分组的书数", rows[0].Count, 1)
	requireEqual(t, "第二个分组的大分类", rows[1].L1, "玄幻")
	requireEqual(t, "第二个分组的书数", rows[1].Count, 2)
}

func TestUpdateBookMetaCategory(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := sampleBook("改分类", "作者甲", "/books/改分类.txt", strings.Repeat("1", 64))
	res, err := st.SaveBook(ctx, b, SaveOptions{})
	requireOK(t, "SaveBook", err)

	l1, l2 := "  仙侠  ", "修真文明"
	err = st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{CategoryL1: &l1, CategoryL2: &l2})
	requireOK(t, "UpdateBookMeta", err)

	got, err := st.GetBook(ctx, res.BookID)
	requireOK(t, "GetBook", err)
	requireEqual(t, "大分类去空白", got.CategoryL1, "仙侠")
	requireEqual(t, "小分类", got.CategoryL2, "修真文明")

	empty := ""
	err = st.UpdateBookMeta(ctx, res.BookID, BookMetaPatch{CategoryL1: &empty, CategoryL2: &empty})
	requireOK(t, "清空分类", err)
	got, err = st.GetBook(ctx, res.BookID)
	requireOK(t, "GetBook", err)
	requireEqual(t, "清空后大分类", got.CategoryL1, "")
}
