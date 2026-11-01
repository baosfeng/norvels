package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"norvels/internal/model"
)

// 本文件锁死章级去重判定（设计 §4）的行为：
//   - SaveBook 成功提交后把本书指纹写进正文库，覆盖/追更时先删旧行；
//   - CheckContentDuplicate 按字节加权算命中率、按命中书目判重、按批（>500）查库；
//   - 指纹表缺失（0019 未应用）时退化为「不判重」而不是让导入失败。
//
// 语料一律合成（虚构书名 + 造出来的正文），不碰真实库与真实书名。

// syntheticChapters 造 n 章合成正文：每章内容不同，长度接近（便于断言字节）。
func syntheticChapters(n int) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, n)
	for i := 0; i < n; i++ {
		body := fmt.Sprintf("第%d章：梅宁城的风从城楼吹过，少年按住剑柄，远处的雪线压得很低。", i)
		out = append(out, model.ParsedChapter{Idx: i, VolumeIdx: -1, Title: fmt.Sprintf("第%d章", i+1), Content: body})
	}
	return out
}

// noisyChapters 给每章插进空白与全角半角差异：内容不变、字节变了（模拟换行/转码差异）。
func noisyChapters(chapters []model.ParsedChapter) []model.ParsedChapter {
	out := make([]model.ParsedChapter, 0, len(chapters))
	for _, c := range chapters {
		noisy := strings.ReplaceAll(c.Content, "，", "，\n")
		noisy = strings.ReplaceAll(noisy, "第", " 第 ")
		out = append(out, model.ParsedChapter{Idx: c.Idx, VolumeIdx: c.VolumeIdx, Title: c.Title, Content: noisy})
	}
	return out
}

// syntheticBook 把章节包成一本可入库的书（SourcePath / SHA256 必填）。
func syntheticBook(title, path, sha string, chapters []model.ParsedChapter) *model.ParsedBook {
	return &model.ParsedBook{
		Title:      title,
		Author:     "示例作者",
		SourcePath: path,
		SourceName: path,
		Encoding:   "utf-8",
		SHA256:     sha,
		Chapters:   chapters,
	}
}

// fingerprintRows 返回一本书在正文库里的指纹行数。
func fingerprintRows(t *testing.T, ctx context.Context, st *Store, bookID int64) int {
	t.Helper()
	var n int
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_fingerprints WHERE book_id = ?", bookID).Scan(&n); err != nil {
		t.Fatalf("数指纹行 book=%d: %v", bookID, err)
	}
	return n
}

// TestSaveBookWritesFingerprints SaveBook 提交后本书指纹落正文库，字节与章节一致。
func TestSaveBookWritesFingerprints(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(5)
	book := syntheticBook("指纹之书", "/books/fp.txt", strings.Repeat("f", 64), chapters)
	res := saveOrFail(t, ctx, st, book)

	if got := fingerprintRows(t, ctx, st, res.BookID); got != len(chapters) {
		t.Fatalf("指纹行 = %d，期望 %d", got, len(chapters))
	}
	hash, bytes, ok := chapterFingerprint(chapters[0].Content)
	if !ok {
		t.Fatal("样例章节不该是空章")
	}
	var stored int64
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT bytes FROM chapter_fingerprints WHERE hash = ? AND book_id = ?", hash, res.BookID).Scan(&stored); err != nil {
		t.Fatalf("读指纹字节: %v", err)
	}
	if stored != bytes || stored != int64(len(chapters[0].Content)) {
		t.Errorf("指纹字节 = %d，期望章节原始字节 %d", stored, bytes)
	}
}

// TestSaveBookSkipsEmptyChapterFingerprint 空白章留指纹没有意义：不写行、不计字节。
func TestSaveBookSkipsEmptyChapterFingerprint(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(3)
	chapters = append(chapters, model.ParsedChapter{Idx: 3, VolumeIdx: -1, Title: "空章", Content: " \n\u3000"})
	book := syntheticBook("留白之书", "/books/blank.txt", strings.Repeat("b", 64), chapters)
	res := saveOrFail(t, ctx, st, book)

	if got := fingerprintRows(t, ctx, st, res.BookID); got != 3 {
		t.Fatalf("指纹行 = %d，期望 3（空章不写）", got)
	}
	rep, err := st.CheckContentDuplicate(ctx, chapters)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.ChapterCount != 3 {
		t.Errorf("参与判定章节 = %d，期望 3（空章不计）", rep.ChapterCount)
	}
}

// TestSaveBookOverwriteReplacesFingerprints 覆盖导入后旧指纹必须消失（否则新书会被自己的旧版本判成重复）。
func TestSaveBookOverwriteReplacesFingerprints(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(5)
	book := syntheticBook("追更之书", "/books/up.txt", strings.Repeat("u", 64), chapters)
	res := saveOrFail(t, ctx, st, book)
	oldHash, _, _ := chapterFingerprint(chapters[0].Content)

	// 覆盖成只剩 2 章（内容全新）：旧指纹必须一条不剩。
	fresh := syntheticChapters(2)
	for i := range fresh {
		fresh[i].Content = "覆盖后的全新正文：" + fresh[i].Content
	}
	overflow := syntheticBook("追更之书", "/books/up.txt", strings.Repeat("u", 64), fresh)
	if _, err := st.SaveBook(ctx, overflow, SaveOptions{Force: true}); err != nil {
		t.Fatalf("覆盖保存: %v", err)
	}
	if got := fingerprintRows(t, ctx, st, res.BookID); got != len(fresh) {
		t.Errorf("覆盖后指纹行 = %d，期望 %d", got, len(fresh))
	}
	var n int
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_fingerprints WHERE hash = ? AND book_id = ?", oldHash, res.BookID).Scan(&n); err != nil {
		t.Fatalf("查旧指纹: %v", err)
	}
	if n != 0 {
		t.Error("覆盖后旧章节指纹仍在（判重会拿自己的旧版本当重复）")
	}
}

// judgedFixture 存一本 10 章的合成书，返回库、上下文、章节与保存结果（三档判定共用）。
func judgedFixture(t *testing.T) (*Store, context.Context, []model.ParsedChapter, SaveResult) {
	t.Helper()
	ctx := context.Background()
	st := newTestStore(t)
	chapters := syntheticChapters(10)
	book := syntheticBook("判定之书", "/books/judge.txt", strings.Repeat("j", 64), chapters)
	return st, ctx, chapters, saveOrFail(t, ctx, st, book)
}

// TestCheckContentDuplicateExactCopy 纯重复（只有排版差异）：100% 命中，分档是「同一本书」。
func TestCheckContentDuplicateExactCopy(t *testing.T) {
	st, ctx, chapters, res := judgedFixture(t)
	rep, err := st.CheckContentDuplicate(ctx, noisyChapters(chapters))
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.HitRate != 1 || rep.DuplicateBookID != res.BookID || rep.DuplicateChapters != len(chapters) {
		t.Fatalf("报告 = %+v，期望 100%% 命中书 %d", rep, res.BookID)
	}
	if rep.DuplicateBookTitle != "判定之书" {
		t.Errorf("命中书书名 = %q，期望 判定之书", rep.DuplicateBookTitle)
	}
	if level := rep.Level(DefaultDedupeThreshold); level != DedupeLevelDuplicate {
		t.Errorf("分档 = %q，期望 %q", level, DedupeLevelDuplicate)
	}
}

// TestCheckContentDuplicatePartial 改 10%：命中率落在 [40%, 阈值) —— 分档「疑似相关」，
// 阈值降到 0.5 时同一份报告改判「同一本书」（分档只由阈值决定）。
func TestCheckContentDuplicatePartial(t *testing.T) {
	st, ctx, chapters, _ := judgedFixture(t)
	mixed := make([]model.ParsedChapter, 0, len(chapters))
	mixed = append(mixed, chapters...)
	mixed[0].Content = "这一章被彻底改写：风雪、孤城、另一个结局。"
	rep, err := st.CheckContentDuplicate(ctx, mixed)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.HitRate <= 0.4 || rep.HitRate >= DefaultDedupeThreshold {
		t.Fatalf("命中率 = %.3f，期望落在 [0.4, 0.95) 之间", rep.HitRate)
	}
	if level := rep.Level(DefaultDedupeThreshold); level != DedupeLevelRelated {
		t.Errorf("分档 = %q，期望 %q", level, DedupeLevelRelated)
	}
	if level := rep.Level(0.5); level != DedupeLevelDuplicate {
		t.Errorf("阈值 0.5 时分档 = %q，期望按阈值判成 %q", level, DedupeLevelDuplicate)
	}
}

// TestCheckContentDuplicateNewBook 内容全新的书：零命中、分档「新书」。
func TestCheckContentDuplicateNewBook(t *testing.T) {
	st, ctx, _, _ := judgedFixture(t)
	fresh := syntheticChapters(8)
	for i := range fresh {
		fresh[i].Content = fmt.Sprintf("另一本全新的书，第%d章：孤舟泊岸，灯火在雨里摇晃。", i)
	}
	rep, err := st.CheckContentDuplicate(ctx, fresh)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.HitRate != 0 || rep.DuplicateBookID != 0 {
		t.Fatalf("报告 = %+v，期望无命中", rep)
	}
	if level := rep.Level(DefaultDedupeThreshold); level != DedupeLevelNovel {
		t.Errorf("分档 = %q，期望 %q", level, DedupeLevelNovel)
	}
}

// TestCheckContentDuplicateEmptyInput 空输入不报错、不判重（不能除零，也不能给空书编一个命中率）。
func TestCheckContentDuplicateEmptyInput(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, chapters := range [][]model.ParsedChapter{
		nil,
		{},
		{{Idx: 0, Content: " \n\t"}},
	} {
		rep, err := st.CheckContentDuplicate(ctx, chapters)
		if err != nil {
			t.Fatalf("CheckContentDuplicate: %v", err)
		}
		if rep.TotalBytes != 0 || rep.HitRate != 0 || rep.DuplicateBookID != 0 {
			t.Errorf("报告 = %+v，期望全零", rep)
		}
		if level := rep.Level(DefaultDedupeThreshold); level != DedupeLevelNovel {
			t.Errorf("分档 = %q，期望 %q", level, DedupeLevelNovel)
		}
	}
}

// TestCheckContentDuplicateBatches 超过一批（500）的哈希必须全部查到：1200 章跨 3 批。
func TestCheckContentDuplicateBatches(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	const chapters = 1200
	fps := syntheticChapters(chapters)
	st.saveBookFingerprints(ctx, 7, fps)

	rep, err := st.CheckContentDuplicate(ctx, fps)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.HitRate != 1 || rep.DuplicateChapters != chapters {
		t.Fatalf("报告 = %+v，期望 %d 章全部命中", rep, chapters)
	}
	if rep.DuplicateBookTitle != "" {
		t.Errorf("书 7 不存在于主库，书名应为空，实际 %q", rep.DuplicateBookTitle)
	}
}

// TestCheckContentDuplicatePicksTopBook 多本命中时取命中字节最多的；并列取 id 小的。
func TestCheckContentDuplicatePicksTopBook(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	fps := syntheticChapters(10)
	st.saveBookFingerprints(ctx, 101, fps)     // 全书 10 章
	st.saveBookFingerprints(ctx, 102, fps[4:]) // 后 6 章

	rep, err := st.CheckContentDuplicate(ctx, fps)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.DuplicateBookID != 101 || rep.DuplicateChapters != 10 {
		t.Fatalf("命中最多的书 = %d（%d 章），期望 101（10 章）", rep.DuplicateBookID, rep.DuplicateChapters)
	}

	// 只判后 6 章：两本书都命中 6 章 → 并列，按 id 小的取胜（结果可复现）。
	tie, err := st.CheckContentDuplicate(ctx, fps[4:])
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if tie.DuplicateBookID != 101 {
		t.Errorf("并列时应取 id 小的书 101，实际 %d", tie.DuplicateBookID)
	}
}

// TestCheckContentDuplicateWithoutFingerprintTable 0019 未应用时退化为「不判重」，且不拦导入。
func TestCheckContentDuplicateWithoutFingerprintTable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.cdb.ExecContext(ctx, "DROP TABLE chapter_fingerprints"); err != nil {
		t.Fatalf("删指纹表: %v", err)
	}
	chapters := syntheticChapters(3)
	rep, err := st.CheckContentDuplicate(ctx, chapters)
	if err != nil {
		t.Fatalf("无指纹表时不该报错: %v", err)
	}
	if rep.HitRate != 0 || rep.TotalBytes == 0 {
		t.Errorf("报告 = %+v，期望『有内容但无命中』", rep)
	}
	// SaveBook 仍要成功（指纹写不进去只记 WARN）。
	if _, err := st.SaveBook(ctx, syntheticBook("无表之书", "/books/nofp.txt", strings.Repeat("n", 64), chapters), SaveOptions{}); err != nil {
		t.Fatalf("指纹表缺失时 SaveBook: %v", err)
	}
}

// TestFingerprintQueryUsesPrimaryKey 「按 hash 查」走主键：WITHOUT ROWID 表的聚簇主键就是索引，
// 因此不另建 (hash) 索引；这条计划是「没有冗余索引」的证据。
func TestFingerprintQueryUsesPrimaryKey(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	rows, err := st.cdb.QueryContext(ctx,
		"EXPLAIN QUERY PLAN SELECT hash, book_id, bytes FROM chapter_fingerprints WHERE hash IN (?, ?)", 1, 2)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close() //nolint:errcheck // 只读句柄
	var detail string
	for rows.Next() {
		var id, parent, notused int
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("扫描计划: %v", err)
		}
	}
	if !strings.Contains(detail, "PRIMARY KEY") {
		t.Errorf("按 hash 查的计划 = %q，期望走 PRIMARY KEY", detail)
	}
	// 按书删（覆盖导入）要有索引：主键以 hash 打头，不建这条就是全表扫。
	var n int
	if err := st.cdb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_chapter_fingerprints_book'").Scan(&n); err != nil {
		t.Fatalf("查索引: %v", err)
	}
	if n != 1 {
		t.Error("缺 idx_chapter_fingerprints_book（按书删旧指纹会退化全表扫）")
	}
}

// TestCheckContentDuplicate40MB 是性能契约的证据：判定一本 40 MB / 2000 章的书 < 200 ms。
func TestCheckContentDuplicate40MB(t *testing.T) {
	if testing.Short() {
		t.Skip("性能用例要造 40 MB 合成语料")
	}
	if testing.CoverMode() != "" {
		// 覆盖率插桩会把每个 rune 循环放大数倍，插桩下计时没有意义（门禁 7 就跑这个口径）。
		// 性能契约的证据在非插桩运行里：go test ./internal/store -run TestCheckContentDuplicate40MB。
		t.Skip("覆盖率插桩下不计时")
	}
	ctx := context.Background()
	st := newTestStore(t)
	para := "梅宁城的风从城楼吹过，少年按住剑柄，远处的雪线压得很低。\n"
	const (
		chapterCount = 2000
		chapterBytes = 21 << 10 // 单章 21 KB × 2000 章 ≥ 40 MiB（口径见设计 §4）
	)
	body := strings.Repeat(para, chapterBytes/len(para)+1)
	chapters := make([]model.ParsedChapter, 0, chapterCount)
	for i := 0; i < chapterCount; i++ {
		chapters = append(chapters, model.ParsedChapter{
			Idx: i, VolumeIdx: -1, Title: fmt.Sprintf("第%d章", i+1),
			Content: fmt.Sprintf("第%d章\n%s", i, body),
		})
	}
	total := 0
	for i := range chapters {
		total += len(chapters[i].Content)
	}
	if total < 40<<20 {
		t.Fatalf("语料只有 %d 字节，达不到 40 MB 的判定口径", total)
	}
	st.saveBookFingerprints(ctx, 1, chapters)

	start := time.Now()
	rep, err := st.CheckContentDuplicate(ctx, chapters)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("CheckContentDuplicate: %v", err)
	}
	if rep.HitRate != 1 || rep.ChapterCount != chapterCount {
		t.Fatalf("报告 = %+v，期望 %d 章全部命中", rep, chapterCount)
	}
	t.Logf("判定 %d 章 / %d MB：%s", chapterCount, total>>20, elapsed)
	if limit := 200 * time.Millisecond; elapsed > limit {
		t.Errorf("判定耗时 %s，超过 %s 的性能契约", elapsed, limit)
	}
}
