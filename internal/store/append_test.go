package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"norvels/internal/model"
)

// serialBook 造一本「章号连续、标题可预测」的书，便于断言前缀匹配。
// WordCount 留 0：走 store 的「按章节重算」路径，顺带覆盖该口径。
func serialBook(title, path, sha string, chapters int) *model.ParsedBook {
	b := &model.ParsedBook{
		Title:      title,
		Author:     "示例作者",
		SourcePath: path,
		SourceName: filepath.Base(path),
		Encoding:   "utf-8",
		FileSize:   int64(chapters) * 128,
		SHA256:     sha,
	}
	for i := 0; i < chapters; i++ {
		b.Chapters = append(b.Chapters, model.ParsedChapter{
			Idx:       i,
			VolumeIdx: -1,
			Title:     fmt.Sprintf("第%d章 示例", i+1),
			Content:   strings.Repeat("正文", i+1),
			WordCount: 2 * (i + 1),
		})
	}
	return b
}

// chapterIDs 按目录顺序返回某本书的章节 id。
func chapterIDs(t *testing.T, st *Store, ctx context.Context, bookID int64) []int64 {
	t.Helper()
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: bookID, From: 1, Limit: 500})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	out := make([]int64, 0, len(page.Items))
	for _, c := range page.Items {
		out = append(out, c.ID)
	}
	return out
}

// TestSaveBookUpdateAppendsTail 追更主路径：只追加尾部章节，旧章节 id 一个不变。
func TestSaveBookUpdateAppendsTail(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.SaveBook(ctx, serialBook("追更示例书", "/path/to/serial.txt", strings.Repeat("a", 64), 10), SaveOptions{})
	requireNoErr(t, err, "首次导入")
	before := chapterIDs(t, st, ctx, first.BookID)
	if len(before) != 10 {
		t.Fatalf("首次导入章节数 = %d，期望 10", len(before))
	}

	// 文件长到 15 章：路径不变，内容变了（sha 不同）。
	grown := serialBook("追更示例书", "/path/to/serial.txt", strings.Repeat("b", 64), 15)
	upd, err := st.SaveBook(ctx, grown, SaveOptions{Update: true})
	requireNoErr(t, err, "追更")
	if upd.BookID != first.BookID || !upd.Updated || upd.Skipped {
		t.Fatalf("追更结果 = %+v，期望同一本书 Updated=true", upd)
	}
	requireChapterIDsKept(t, before, chapterIDs(t, st, ctx, first.BookID))
	// 字数按章节重算：2*(1+…+15) = 240。
	requireBookCounts(t, st, ctx, first.BookID, 15, 240)
}

// requireNoErr 是测试里的「这一步不该失败」断言。
func requireNoErr(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// requireChapterIDsKept 断言 after 的前 len(before) 章 id 与 before 逐一相同（追更不换 id）。
func requireChapterIDsKept(t *testing.T, before, after []int64) {
	t.Helper()
	if len(after) < len(before) {
		t.Fatalf("章节数从 %d 掉到 %d", len(before), len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Fatalf("第 %d 章 id 由 %d 变成 %d（旧章节 id 必须不变）", i+1, before[i], after[i])
		}
	}
}

// requireBookCounts 断言书级计数（章节数 / 字数）。
func requireBookCounts(t *testing.T, st *Store, ctx context.Context, bookID int64, chapters, words int) {
	t.Helper()
	detail, err := st.GetBook(ctx, bookID)
	requireNoErr(t, err, "GetBook")
	if detail.ChapterCount != chapters || detail.WordCount != words {
		t.Fatalf("章节数/字数 = %d/%d，期望 %d/%d", detail.ChapterCount, detail.WordCount, chapters, words)
	}
}

// TestSaveBookUpdateKeepsReadingProgress 追更后 progress 指向的章节仍然可读。
func TestSaveBookUpdateKeepsReadingProgress(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.SaveBook(ctx, serialBook("进度示例书", "/path/to/progress.txt", strings.Repeat("c", 64), 3), SaveOptions{})
	if err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	ids := chapterIDs(t, st, ctx, first.BookID)
	if err := st.SaveReadingProgress(ctx, ReadingProgress{
		BookID: first.BookID, ChapterID: ids[2], ChapterIdx: 2, Percent: 0.5,
	}); err != nil {
		t.Fatalf("SaveReadingProgress: %v", err)
	}

	if _, err := st.SaveBook(ctx, serialBook("进度示例书", "/path/to/progress.txt", strings.Repeat("d", 64), 5),
		SaveOptions{Update: true}); err != nil {
		t.Fatalf("追更失败: %v", err)
	}

	requireProgressKept(t, st, ctx, first.BookID, ids[2])
}

// requireProgressKept 断言阅读进度仍指向同一个可读章节（chapter_id 没悬空）。
func requireProgressKept(t *testing.T, st *Store, ctx context.Context, bookID, wantChapterID int64) {
	t.Helper()
	items, err := st.ListReadingProgress(ctx, 5)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListReadingProgress = %+v, err=%v", items, err)
	}
	kept := items[0]
	if kept.ChapterID != wantChapterID {
		t.Fatalf("进度章节 id = %d，期望仍是 %d", kept.ChapterID, wantChapterID)
	}
	if kept.ChapterTitle == "" {
		t.Error("进度记录联表拿不到章节标题：chapter_id 已悬空")
	}
	ch, err := st.GetChapter(ctx, bookID, wantChapterID)
	if err != nil || ch == nil || ch.Content == "" {
		t.Fatalf("追更后读取原进度章节失败: ch=%+v err=%v", ch, err)
	}
}

// TestSaveBookUpdateRejectsNonPrefix 标题改了就不是同一序列：报 ErrNotAppendable 且不落库。
func TestSaveBookUpdateRejectsNonPrefix(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.SaveBook(ctx, serialBook("改题示例书", "/path/to/rename.txt", strings.Repeat("e", 64), 3), SaveOptions{})
	if err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	changed := serialBook("改题示例书", "/path/to/rename.txt", strings.Repeat("f", 64), 4)
	changed.Chapters[1].Title = "第2章 改过的标题"

	if _, err := st.SaveBook(ctx, changed, SaveOptions{Update: true}); !errors.Is(err, ErrNotAppendable) {
		t.Fatalf("错误 = %v，期望 ErrNotAppendable", err)
	}
	if got := len(chapterIDs(t, st, ctx, first.BookID)); got != 3 {
		t.Fatalf("被拒的追更改了库：章节数 = %d，期望 3", got)
	}
}

// TestSaveBookUpdateRejectsShorter 章节变少（文件被截断/换了本更短的书）必须拒绝，不能截断。
func TestSaveBookUpdateRejectsShorter(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.SaveBook(ctx, serialBook("截断示例书", "/path/to/shorten.txt", strings.Repeat("g", 64), 5), SaveOptions{})
	if err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	shorter := serialBook("截断示例书", "/path/to/shorten.txt", strings.Repeat("h", 64), 3)

	if _, err := st.SaveBook(ctx, shorter, SaveOptions{Update: true}); !errors.Is(err, ErrNotAppendable) {
		t.Fatalf("错误 = %v，期望 ErrNotAppendable", err)
	}
	if got := len(chapterIDs(t, st, ctx, first.BookID)); got != 5 {
		t.Fatalf("章节数 = %d，期望仍是 5（拒绝截断）", got)
	}
}

// TestSaveBookUpdateMatchesByTitleAuthor 换了路径也追更到同一本书（第三级匹配兜底）。
func TestSaveBookUpdateMatchesByTitleAuthor(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.SaveBook(ctx, serialBook("换路径示例书", "/path/to/old.txt", strings.Repeat("i", 64), 3), SaveOptions{})
	if err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	moved := serialBook("换路径示例书", "/path/to/new.txt", strings.Repeat("j", 64), 4)
	upd, err := st.SaveBook(ctx, moved, SaveOptions{Update: true})
	if err != nil {
		t.Fatalf("按书名作者追更失败: %v", err)
	}
	if upd.BookID != first.BookID {
		t.Fatalf("追更落到书 %d，期望 %d", upd.BookID, first.BookID)
	}
	if got := len(chapterIDs(t, st, ctx, first.BookID)); got != 4 {
		t.Fatalf("章节数 = %d，期望 4", got)
	}
}

// TestSaveBookUpdateSameChaptersIsNoop 同一份文件再导一次：不重复插章节，id 全不变。
func TestSaveBookUpdateSameChaptersIsNoop(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	book := serialBook("幂等示例书", "/path/to/same.txt", strings.Repeat("k", 64), 4)
	first, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	before := chapterIDs(t, st, ctx, first.BookID)

	if _, err := st.SaveBook(ctx, book, SaveOptions{Update: true}); err != nil {
		t.Fatalf("重复追更: %v", err)
	}
	after := chapterIDs(t, st, ctx, first.BookID)
	if len(after) != len(before) {
		t.Fatalf("重复追更插出了新章节：%d → %d", len(before), len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Fatalf("第 %d 章 id 变了：%d → %d", i+1, before[i], after[i])
		}
	}
}

// TestSaveBookUpdateCreatesBookWhenMissing 追更没命中任何已有书时按普通导入新建。
func TestSaveBookUpdateCreatesBookWhenMissing(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	res, err := st.SaveBook(ctx, serialBook("新书示例书", "/path/to/brand-new.txt", strings.Repeat("l", 64), 2), SaveOptions{Update: true})
	if err != nil {
		t.Fatalf("追更新建失败: %v", err)
	}
	if res.Updated || res.BookID <= 0 {
		t.Fatalf("结果 = %+v，期望新建（Updated=false）", res)
	}
}

// TestSaveBookUpdateAppendsIntoNewVolume 追更追加的章节落在新卷里：建行、volume_count 跟着涨。
func TestSaveBookUpdateAppendsIntoNewVolume(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first := serialBook("追更分卷示例书", "/path/to/vol.txt", strings.Repeat("y", 64), 2)
	first.Volumes = []model.ParsedVolume{{Idx: 0, Title: "第一卷"}}
	for i := range first.Chapters {
		first.Chapters[i].VolumeIdx = 0
	}
	res, err := st.SaveBook(ctx, first, SaveOptions{})
	requireNoErr(t, err, "首次导入")

	grown := serialBook("追更分卷示例书", "/path/to/vol.txt", strings.Repeat("z", 64), 4)
	grown.Volumes = []model.ParsedVolume{{Idx: 0, Title: "第一卷"}, {Idx: 1, Title: "第二卷"}}
	for i := range grown.Chapters {
		if i >= 2 {
			grown.Chapters[i].VolumeIdx = 1
		} else {
			grown.Chapters[i].VolumeIdx = 0
		}
	}
	if _, err := st.SaveBook(ctx, grown, SaveOptions{Update: true}); err != nil {
		t.Fatalf("追更（新增卷）失败: %v", err)
	}

	detail, err := st.GetBook(ctx, res.BookID)
	requireNoErr(t, err, "GetBook")
	if detail.VolumeCount != 2 || len(detail.Volumes) != 2 {
		t.Fatalf("卷数 = %d（详情 %d 卷），期望 2", detail.VolumeCount, len(detail.Volumes))
	}
	if len(detail.Volumes[1].Chapters) != 2 || detail.Volumes[1].Title != "第二卷" {
		t.Fatalf("第二卷 = %+v，期望 2 章", detail.Volumes[1])
	}
}

// TestSaveBookUpdateKeepsSHAWhenTaken 新文件的哈希已被另一本书占用时，追更仍要成功（保留旧哈希）。
//
// file_sha256 上有 UNIQUE：如果直接覆盖哈希，这里会变成约束错误、整次追更失败；
// 计数照更新、哈希保持旧值才是「不因一个展示字段挡住主流程」的处置。
func TestSaveBookUpdateKeepsSHAWhenTaken(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	shared := strings.Repeat("s", 64)
	first, err := st.SaveBook(ctx, serialBook("哈希冲突示例书", "/path/to/hash-a.txt", strings.Repeat("A", 64), 2), SaveOptions{})
	requireNoErr(t, err, "首次导入")
	other, err := st.SaveBook(ctx, serialBook("占用哈希示例书", "/path/to/hash-b.txt", shared, 2), SaveOptions{})
	requireNoErr(t, err, "写入另一本书")

	grown := serialBook("哈希冲突示例书", "/path/to/hash-a.txt", shared, 3)
	if _, err := st.SaveBook(ctx, grown, SaveOptions{Update: true}); err != nil {
		t.Fatalf("哈希被占用时追更失败: %v", err)
	}
	requireBookCounts(t, st, ctx, first.BookID, 3, 12)
	var otherSHA string
	if err := st.DB().QueryRowContext(ctx, "SELECT file_sha256 FROM books WHERE id = ?", other.BookID).Scan(&otherSHA); err != nil {
		t.Fatalf("查另一本书的哈希: %v", err)
	}
	if otherSHA != shared {
		t.Fatalf("另一本书的哈希被改动: %q", otherSHA)
	}
}
