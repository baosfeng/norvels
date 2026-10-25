package importer

import (
	"context"
	"testing"

	"norvels/internal/store"
)

// 追更链路的口径：importer 在 SaveBook 返回 Appended>0 后累加 books.new_chapters，
// 没有新章时不涨；角标写失败不打断导入（这里用正常路径覆盖主行为）。

func TestRunUpdateAccumulatesNewChapterBadge(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	dir := t.TempDir()
	writeTxt(t, dir, "示例书.txt", sampleText("示例书", "示例作者"))

	if _, err := Run(ctx, st, opts(dir), nil); err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	bookID := onlyBookID(t, ctx, st)

	// 同一个文件追加第四章后重跑：源路径命中既有书，走追更。
	writeTxt(t, dir, "示例书.txt",
		sampleText("示例书", "示例作者")+"第四章 归来\n"+bodyA+bodyA)
	opt := opts(dir)
	opt.Update = true
	if _, err := Run(ctx, st, opt, nil); err != nil {
		t.Fatalf("追更导入: %v", err)
	}
	detail, err := st.GetBook(ctx, bookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if detail.ChapterCount != 4 {
		t.Errorf("追更后章节数 = %d，期望 4", detail.ChapterCount)
	}
	if detail.NewChapters != 1 {
		t.Errorf("追更角标 = %d，期望 1（本次追加 1 章）", detail.NewChapters)
	}

	// 内容没变再追更一次：Appended=0，角标不该继续涨。
	if _, err := Run(ctx, st, opt, nil); err != nil {
		t.Fatalf("空追更导入: %v", err)
	}
	again, err := st.GetBook(ctx, bookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if again.NewChapters != 1 || again.ChapterCount != 4 {
		t.Errorf("空追更后 角标=%d 章数=%d，期望 1/4", again.NewChapters, again.ChapterCount)
	}
}

// onlyBookID 返回库里唯一一本书的 id。
func onlyBookID(t *testing.T, ctx context.Context, st *store.Store) int64 {
	t.Helper()
	page, err := st.ListBooks(ctx, store.BookQuery{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("库里书数 = %d，期望 1", page.Total)
	}
	return page.Items[0].ID
}
