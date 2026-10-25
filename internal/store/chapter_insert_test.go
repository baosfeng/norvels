package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/model"
)

// 新建章节与追更角标的口径：插到指定章之后（0 = 末尾）、idx 顺延并重排、
// 正文走压缩块、三处计数同事务；追更角标只由追更链路累加、ack 清零、负数拒绝。

func TestInsertChapterAppend(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)
	before := mustBookDetail(t, ctx, st, bookID)

	content := "新增的一章正文，用来验证压缩块写入与计数刷新。"
	chapterID, err := st.InsertChapter(ctx, bookID, model.ParsedChapter{
		Title: "第四章 新增", Content: content,
	}, 0)
	if err != nil {
		t.Fatalf("InsertChapter 追加: %v", err)
	}
	after := mustBookDetail(t, ctx, st, bookID)
	if after.ChapterCount != before.ChapterCount+1 {
		t.Errorf("章节数 = %d，期望 %d", after.ChapterCount, before.ChapterCount+1)
	}
	wantWords := before.WordCount + countWords(content)
	if after.WordCount != wantWords {
		t.Errorf("字数 = %d，期望 %d", after.WordCount, wantWords)
	}
	got, err := st.GetChapter(ctx, bookID, chapterID)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if got.Idx != before.ChapterCount || got.Title != "第四章 新增" || got.Content != content {
		t.Errorf("新章 = %+v，期望 idx=%d 且正文原样可读", got, before.ChapterCount)
	}
	if got.WordCount != countWords(content) {
		t.Errorf("新章字数 = %d，期望 %d", got.WordCount, countWords(content))
	}
	requireChapterIdxContiguous(t, ctx, st, bookID)
}

func TestInsertChapterAfterMiddle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)
	ids := chapterIDsOf(t, ctx, st, bookID)
	var beforeVolume int64
	if err := st.DB().QueryRowContext(ctx,
		"SELECT volume_id FROM chapters WHERE id = ?", ids[1]).Scan(&beforeVolume); err != nil {
		t.Fatalf("读卷归属: %v", err)
	}

	chapterID, err := st.InsertChapter(ctx, bookID, model.ParsedChapter{
		Title: "插在第二章之后", Content: "插队的正文。",
	}, ids[1])
	if err != nil {
		t.Fatalf("InsertChapter 插入: %v", err)
	}
	if got := titlesOf(t, ctx, st, bookID); fmt.Sprint(got) != "[第一章 风起 第二章 云涌 插在第二章之后 番外 小记]" {
		t.Errorf("目录顺序 = %v，期望新章排在第二章之后", got)
	}
	var volumeID int64
	var idx int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT volume_id, idx FROM chapters WHERE id = ?", chapterID).Scan(&volumeID, &idx); err != nil {
		t.Fatalf("读新章: %v", err)
	}
	if idx != 2 || volumeID != beforeVolume {
		t.Errorf("新章 idx=%d volume=%d，期望 idx=2 且继承卷 %d", idx, volumeID, beforeVolume)
	}
	requireChapterIdxContiguous(t, ctx, st, bookID)

	// 追加到末尾：落点 = 现有章数，卷继承最后一章。
	lastID := chapterIDsOf(t, ctx, st, bookID)[len(ids)]
	if _, err := st.InsertChapter(ctx, bookID, model.ParsedChapter{
		Title: "末尾追加", Content: "末尾正文。",
	}, lastID); err != nil {
		t.Fatalf("InsertChapter 末尾: %v", err)
	}
	final := mustBookDetail(t, ctx, st, bookID)
	if final.ChapterCount != len(ids)+2 {
		t.Errorf("末尾追加后章节数 = %d，期望 %d", final.ChapterCount, len(ids)+2)
	}
	requireChapterIdxContiguous(t, ctx, st, bookID)
}

func TestInsertChapterErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)
	otherBook := seedInsertBook(t, ctx, st, 2)
	otherChapter := chapterIDsOf(t, ctx, st, otherBook)[0]
	good := model.ParsedChapter{Title: "合法标题", Content: "合法正文。"}

	if _, err := st.InsertChapter(ctx, 0, good, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("book=0 err = %v", err)
	}
	if _, err := st.InsertChapter(ctx, bookID, good, -1); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("afterChapterId=-1 err = %v", err)
	}
	if _, err := st.InsertChapter(ctx, 999999, good, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的书 err = %v，期望 ErrNotFound", err)
	}
	if _, err := st.InsertChapter(ctx, bookID, good, otherChapter); !errors.Is(err, ErrNotFound) {
		t.Errorf("跨书的锚点 err = %v，期望 ErrNotFound", err)
	}
	blankTitle := model.ParsedChapter{Title: "  ", Content: "正文。"}
	if _, err := st.InsertChapter(ctx, bookID, blankTitle, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空标题 err = %v", err)
	}
	emptyBody := model.ParsedChapter{Title: "标题", Content: "  "}
	if _, err := st.InsertChapter(ctx, bookID, emptyBody, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("空正文 err = %v", err)
	}
	huge := model.ParsedChapter{Title: "标题", Content: strings.Repeat("字", 400*1024)}
	if _, err := st.InsertChapter(ctx, bookID, huge, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("超 1 MiB err = %v", err)
	}
}

func TestAddNewChaptersAccumulates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)

	requireOK(t, "AddNewChapters 2", st.AddNewChapters(ctx, bookID, 2))
	requireEqual(t, "角标", mustBookDetail(t, ctx, st, bookID).NewChapters, 2)
	requireOK(t, "AddNewChapters 3", st.AddNewChapters(ctx, bookID, 3))
	requireEqual(t, "累加后角标", mustBookDetail(t, ctx, st, bookID).NewChapters, 5)
	requireOK(t, "AddNewChapters 0 是空操作", st.AddNewChapters(ctx, bookID, 0))
	requireEqual(t, "空操作后角标", mustBookDetail(t, ctx, st, bookID).NewChapters, 5)
}

func TestAckNewChaptersResets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)
	requireOK(t, "AddNewChapters", st.AddNewChapters(ctx, bookID, 2))

	requireOK(t, "AckNewChapters", st.AckNewChapters(ctx, bookID))
	requireEqual(t, "ack 后角标", mustBookDetail(t, ctx, st, bookID).NewChapters, 0)
}

func TestNewChaptersErrors(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedInsertBook(t, ctx, st, 1)

	requireErr(t, "负数", st.AddNewChapters(ctx, bookID, -1), ErrInvalidArgument)
	requireErr(t, "不存在的书", st.AddNewChapters(ctx, 999999, 1), ErrNotFound)
	requireErr(t, "ack 不存在的书", st.AckNewChapters(ctx, 999999), ErrNotFound)
	requireErr(t, "ack id=0", st.AckNewChapters(ctx, 0), ErrInvalidArgument)
}

// TestSaveBookUpdateReportsAppended 追更时库层已算过 tail 长度，上层不再自己数。
func TestSaveBookUpdateReportsAppended(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.SaveBook(ctx, seedInsertBookModel(1), SaveOptions{})
	requireOK(t, "首次 SaveBook", err)
	requireEqual(t, "新建 Appended", first.Appended, 0)
	requireEqual(t, "新建不是覆盖", first.Updated, false)

	unchanged, err := st.SaveBook(ctx, seedInsertBookModel(1), SaveOptions{Update: true})
	requireOK(t, "追更（无新章）", err)
	requireEqual(t, "无新章 Appended", unchanged.Appended, 0)
	requireEqual(t, "追更标记 Updated", unchanged.Updated, true)
	requireEqual(t, "命中同一本书", unchanged.BookID, first.BookID)
}

func TestSaveBookUpdateBadge(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	first, err := st.SaveBook(ctx, seedInsertBookModel(1), SaveOptions{})
	requireOK(t, "首次 SaveBook", err)
	before := mustBookDetail(t, ctx, st, first.BookID)

	grown := seedInsertBookModel(1)
	grown.Chapters = append(grown.Chapters, model.ParsedChapter{
		Idx: len(grown.Chapters), VolumeIdx: -1, Title: "第四章 追更", Content: "追更正文。",
	})
	appended, err := st.SaveBook(ctx, grown, SaveOptions{Update: true})
	requireOK(t, "追更", err)
	requireEqual(t, "追更 Appended", appended.Appended, 1)

	// 角标在追更事务内累加，不需要上层再调 AddNewChapters 补一刀。
	detail := mustBookDetail(t, ctx, st, appended.BookID)
	requireEqual(t, "追更后章节数", detail.ChapterCount, 4)
	requireEqual(t, "追更后角标", detail.NewChapters, 1)
	// 追更必须推进 updated_at：书架「最近更新」与详情页「最后更新」都读它，
	// 不推进会让刚追更的书排到旧位置（验收里表现为抓到上一本）。
	if !detail.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("追更后 updatedAt 未前进：之前 %v，之后 %v", before.UpdatedAt, detail.UpdatedAt)
	}
}

// seedInsertBook 造一本示例书（3 章，前两章归第一卷），返回 book id。
func seedInsertBook(t *testing.T, ctx context.Context, st *Store, n int) int64 {
	t.Helper()
	res, err := st.SaveBook(ctx, seedInsertBookModel(n), SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	return res.BookID
}

// seedInsertBookModel 造一本示例书的解析结果（n 用于保证路径/哈希唯一）。
func seedInsertBookModel(n int) *model.ParsedBook {
	book := sampleBook("插章示例书", "示例作者",
		fmt.Sprintf("/path/to/txt/p3-insert-%d.txt", n), fmt.Sprintf("%064d", n))
	book.Chapters[0].Idx, book.Chapters[1].Idx, book.Chapters[2].Idx = 0, 1, 2
	return book
}

// titlesOf 按目录顺序读章节标题。
func titlesOf(t *testing.T, ctx context.Context, st *Store, bookID int64) []string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT title FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		t.Fatalf("读标题: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	var out []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("扫描标题: %v", err)
		}
		out = append(out, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读标题: %v", err)
	}
	return out
}

// requireChapterIdxContiguous 断言目录序号是 0..n-1 连续无空洞。
func requireChapterIdxContiguous(t *testing.T, ctx context.Context, st *Store, bookID int64) {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx,
		"SELECT idx FROM chapters WHERE book_id = ? ORDER BY idx, id", bookID)
	if err != nil {
		t.Fatalf("读 idx: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	want := 0
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			t.Fatalf("扫描 idx: %v", err)
		}
		if idx != want {
			t.Fatalf("章节 idx = %d，期望 %d（必须连续）", idx, want)
		}
		want++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读 idx: %v", err)
	}
}
