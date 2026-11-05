package store

import (
	"context"
	"strings"
	"testing"

	"norvels/internal/model"
)

// TestWritePathsOnClosedDB 库已关闭时，写路径一律返回错误——不能 panic、不能静默成功。
//
// 这是各写入口「打开事务 / 读前置状态」失败分支的兜底契约：连接已关（进程退出、
// 文件被移除）时调用方拿到的是 error，由上层映射成 5xx，而不是让 API 返回 200。
func TestWritePathsOnClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "关库之书", "/path/to/closed.txt", strings.Repeat("f", 64))
	if err := st.SaveCover(ctx, bookID, "image/png", fakePNG); err != nil {
		t.Fatalf("SaveCover: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"SaveCover", func() error { return st.SaveCover(ctx, bookID, "image/png", fakePNG) }},
		{"DeleteCover", func() error { return st.DeleteCover(ctx, bookID) }},
		{"GetCover", func() error { _, err := st.GetCover(ctx, bookID); return err }},
		{"CreateVolume", func() error { _, err := st.CreateVolume(ctx, bookID, "第一卷"); return err }},
		{"DeleteChapter", func() error { return st.DeleteChapter(ctx, bookID, 1) }},
		{"SetBookTags", func() error { return st.SetBookTags(ctx, bookID, []string{"玄幻"}) }},
		{"GetBook", func() error { _, err := st.GetBook(ctx, bookID); return err }},
		{"BookQuality", func() error { _, err := st.BookQuality(ctx, bookID); return err }},
		{"EnterUser", func() error { _, _, err := st.EnterUser(ctx, "关库后的读者"); return err }},
	}
	for _, tc := range cases {
		if err := tc.call(); err == nil {
			t.Errorf("%s 在库关闭时应返回错误", tc.name)
		}
	}
}

// TestMoreWritePathsOnClosedDB 同一契约覆盖其余写入口（导入任务 / 分卷编排 / 合集 /
// 阅读历史）：库关闭后这些入口必须冒泡 error，而不是吞掉失败返回成功。
func TestMoreWritePathsOnClosedDB(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	bookID := seedCoverBook(t, st, ctx, "关库编排之书", "/path/to/closed2.txt", strings.Repeat("a", 64))
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"CreateImportJob", func() error { _, err := st.CreateImportJob(ctx, "/books"); return err }},
		{"UpdateImportJob", func() error { return st.UpdateImportJob(ctx, &ImportJob{ID: 1}) }},
		{"UpsertImportJobFile", func() error { return st.UpsertImportJobFile(ctx, 1, ImportJobFile{Source: "a.txt"}) }},
		{"AddImportError", func() error { return st.AddImportError(ctx, 1, "a.txt", "boom") }},
		{"UpdateChapterTitle", func() error { return st.UpdateChapterTitle(ctx, bookID, 1, "新标题") }},
		{"SaveBook", func() error {
			_, err := st.SaveBook(ctx, sampleBook("关库存书", "示例作者", "/path/to/closed3.txt", strings.Repeat("b", 64)), SaveOptions{})
			return err
		}},
		{"RenameVolume", func() error { _, err := st.RenameVolume(ctx, bookID, 1, "新卷名"); return err }},
		{"DeleteVolume", func() error { return st.DeleteVolume(ctx, bookID, 1) }},
		{"ReorderVolumes", func() error { return st.ReorderVolumes(ctx, bookID, []int64{1}) }},
		{"MoveChaptersToVolume", func() error { _, err := st.MoveChaptersToVolume(ctx, bookID, []int64{1}, 0); return err }},
		{"CreateCollection", func() error { _, err := st.CreateCollection(ctx, "关库合集"); return err }},
		{"RenameCollection", func() error { return st.RenameCollection(ctx, 1, "改名") }},
		{"DeleteCollection", func() error { return st.DeleteCollection(ctx, 1) }},
		{"ReplaceCollectionBooks", func() error { return st.ReplaceCollectionBooks(ctx, 1, []int64{bookID}) }},
		{"DeleteReadingHistory", func() error { return st.DeleteReadingHistory(ctx, bookID) }},
		{"ClearReadingHistory", func() error { return st.ClearReadingHistory(ctx) }},
		{"CheckBookDuplicate", func() error {
			_, err := st.CheckBookDuplicate(ctx, sampleBook("判重之书", "示例作者", "/path/to/dup.txt", strings.Repeat("c", 64)), 0.5)
			return err
		}},
		{"CheckContentDuplicate", func() error {
			_, err := st.CheckContentDuplicate(ctx, []model.ParsedChapter{{Idx: 0, Title: "第1章", Content: "正文"}})
			return err
		}},
		{"BackfillFingerprints", func() error {
			_, err := st.BackfillFingerprints(ctx, FingerprintBackfillOptions{})
			return err
		}},
		{"BackfillQuality", func() error {
			_, err := st.BackfillQuality(ctx, QualityBackfillOptions{})
			return err
		}},
		{"RefreshMetrics", func() error { return st.RefreshMetrics(ctx) }},
		{"MigrateTo", func() error { _, err := st.MigrateTo(ctx, 1); return err }},
	}
	for _, tc := range cases {
		if err := tc.call(); err == nil {
			t.Errorf("%s 在库关闭时应返回错误", tc.name)
		}
	}
}
