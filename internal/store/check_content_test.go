package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 本文件覆盖 check-content 的运维自查能力：抽样/全量、可读性校验、坏块定位。
//
// 这些函数是「迁移之后正文还在不在」的最后一环，必须自己也被测到。

// TestCheckContentAll 全量校验应覆盖每一章且不报错。
func TestCheckContentAll(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	rep, err := st.CheckContent(ctx, ContentCheckOptions{All: true})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if rep.TotalChapters != 6 || rep.Sampled != 6 {
		t.Errorf("校验 %d/%d，期望 6/6", rep.Sampled, rep.TotalChapters)
	}
	if rep.Blocks <= 0 {
		t.Errorf("Blocks = %d，非空库应有压缩块", rep.Blocks)
	}
	if rep.BadUTF8 != 0 {
		t.Errorf("BadUTF8 = %d，正常中文正文应为合法 UTF-8", rep.BadUTF8)
	}
	if len(rep.Corrupted) != 0 {
		t.Errorf("正常库不应有损坏块: %v", rep.Errors())
	}
	if rep.TotalBytes <= 0 || rep.MaxBytes <= 0 {
		t.Errorf("字节统计异常: %+v", rep)
	}
	if rep.MinBytes > rep.MaxBytes {
		t.Errorf("MinBytes(%d) > MaxBytes(%d)", rep.MinBytes, rep.MaxBytes)
	}
}

// TestCheckContentSample 抽查应只覆盖指定数量。
func TestCheckContentSample(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	rep, err := st.CheckContent(ctx, ContentCheckOptions{Sample: 3})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if rep.Sampled != 3 || rep.TotalChapters != 6 {
		t.Errorf("校验 %d/%d，期望 3/6", rep.Sampled, rep.TotalChapters)
	}
}

// TestCheckContentSampleDefaults 未指定抽样数时用默认值（库里不足则全量）。
func TestCheckContentSampleDefaults(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	rep, err := st.CheckContent(ctx, ContentCheckOptions{})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if rep.Sampled != rep.TotalChapters {
		t.Errorf("校验 %d 章，库只有 %d 章时应全量", rep.Sampled, rep.TotalChapters)
	}
}

// TestCheckContentSampleLargerThanTotal 抽样数超过总数时按总数处理。
func TestCheckContentSampleLargerThanTotal(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	seedTwoBooks(t, st)

	rep, err := st.CheckContent(ctx, ContentCheckOptions{Sample: 9999})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if rep.Sampled != rep.TotalChapters {
		t.Errorf("校验 %d，期望全量 %d", rep.Sampled, rep.TotalChapters)
	}
}

// TestCheckContentEmptyDatabase 空库返回全零报告且不报错。
func TestCheckContentEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	rep, err := st.CheckContent(ctx, ContentCheckOptions{All: true})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if rep.TotalChapters != 0 || rep.Sampled != 0 {
		t.Errorf("空库报告 = %+v，期望全零", rep)
	}
}

// TestCheckContentIllegalUTF8IsNotFailure 源数据本身含非法 UTF-8 时，不能判为存储损坏。
//
// 这是真实库踩到的坑：某本书的源 txt 主体 GBK、夹了一段 UTF-8，解析后留下 20 章
// 非法 UTF-8。迁移逐字节保住了它们（正确行为），但 check-content 曾经把它报成
// 「解压或迁移出错」并 exit 1，让用户以为迁移把数据搞坏了。
//
// 正确语义：UTF-8 合法性是**内容质量**，只统计；FAIL 只由块损坏决定。
func TestCheckContentIllegalUTF8IsNotFailure(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	// 直接写一段非法 UTF-8 的正文（模拟源文件留下的脏数据）。
	bad := "正常开头，然后是一段坏字节 \xff\xfe\xfd 后面还有内容。"
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/bad-utf8.txt", repeat64("c"))
	book.Chapters[0].Content = bad
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	rep, err := st.CheckContent(ctx, ContentCheckOptions{All: true})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	// 关键：非法 UTF-8 只计数，不进 Corrupted（不构成存储故障）。
	if rep.BadUTF8 != 1 {
		t.Errorf("BadUTF8 = %d，期望 1", rep.BadUTF8)
	}
	if len(rep.Corrupted) != 0 {
		t.Errorf("非法 UTF-8 不应被算作损坏块: %v", rep.Errors())
	}
	if rep.FailedChapters != 0 {
		t.Errorf("FailedChapters = %d，期望 0（正文读得出来，只是内容不合法）", rep.FailedChapters)
	}
	// 要能报出具体章节 id，供运维反查源文件。
	if len(rep.BadUTF8ChapterIDs) != 1 {
		t.Errorf("BadUTF8ChapterIDs = %v，期望 1 个", rep.BadUTF8ChapterIDs)
	}
}

// TestCheckContentReportsCorruptBlockWithLocation 坏块必须被列出并带定位信息。
//
// 这是运维可用性的关键：几万本书时，一个坏块不能让用户以为整个库坏了，
// 也不能只报「某章读取失败」而不说清楚是哪本书的哪一块。
func TestCheckContentReportsCorruptBlockWithLocation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	// 把块的 packed 换成垃圾字节：解压必然失败。
	if _, err := st.DB().ExecContext(ctx, "UPDATE chapter_blocks SET packed = x'02010203'"); err != nil {
		t.Fatalf("破坏块: %v", err)
	}

	rep, err := st.CheckContent(ctx, ContentCheckOptions{All: true})
	if err != nil {
		t.Fatalf("CheckContent 本身不应报错（损坏通过 Corrupted 表达）: %v", err)
	}
	if len(rep.Corrupted) != 1 {
		t.Fatalf("损坏块数 = %d，期望 1", len(rep.Corrupted))
	}
	assertBlockErrorLocates(t, rep.Corrupted[0], 0, 2)
}

// assertBlockErrorLocates 校验 *BlockError 的定位字段与可 errors.Is 的两个哨兵。
//
// 单独抽出来的原因：这段断言在「check-content 列表」与「GetChapter 单章报错」
// 两个场景都要用，而它们各自还有别的关注点。
func assertBlockErrorLocates(t *testing.T, be *BlockError, wantFirstIdx, wantLastIdx int) {
	t.Helper()
	if be.BookID == 0 || be.Seq == 0 || be.BlockID == 0 {
		t.Errorf("定位信息不全: %+v", be)
	}
	// first_idx/last_idx 是章节序号；本书 3 章（idx 0..2）都在同一块里。
	if be.FirstIdx != wantFirstIdx || be.LastIdx != wantLastIdx {
		t.Errorf("章节范围 = [%d,%d]，期望 [%d,%d]", be.FirstIdx, be.LastIdx, wantFirstIdx, wantLastIdx)
	}
	if !errors.Is(be, ErrBlockCorrupted) {
		t.Errorf("应能 errors.Is 到 ErrBlockCorrupted")
	}
	if !errors.Is(be, ErrCorruptContent) {
		t.Errorf("应能 errors.Is 到 ErrCorruptContent（底层原因）")
	}
	msg := be.Error()
	for _, want := range []string{"book_id=", "block_seq=", "block_id=", "章节 idx"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q：%s", want, msg)
		}
	}
}

// TestCheckContentListsAllCorruptBlocks 多个坏块必须**全部**列出（不能首个即停）。
func TestCheckContentListsAllCorruptBlocks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, suffix := range []string{"a", "b"} {
		b := sampleBook("书"+suffix, "作者", "/path/to/txt/"+suffix+".txt", repeat64(suffix))
		if _, err := st.SaveBook(ctx, b, SaveOptions{}); err != nil {
			t.Fatalf("SaveBook %s: %v", suffix, err)
		}
	}
	// 把所有块都破坏掉（两本书各一块）。
	if _, err := st.DB().ExecContext(ctx, "UPDATE chapter_blocks SET packed = x'02ffff'"); err != nil {
		t.Fatalf("破坏块: %v", err)
	}

	rep, err := st.CheckContent(ctx, ContentCheckOptions{All: true})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if len(rep.Corrupted) != 2 {
		t.Fatalf("损坏块数 = %d，期望 2（必须列全，不能首个即停）", len(rep.Corrupted))
	}
	if rep.Corrupted[0].BookID == rep.Corrupted[1].BookID {
		t.Errorf("两块 book_id 相同，可能重复记录: %+v", rep.Corrupted)
	}
	if len(rep.Errors()) != 2 {
		t.Errorf("Errors() 返回 %d 条，期望 2", len(rep.Errors()))
	}
}

// TestCheckContentSampleModeAlsoDetects 抽样模式也要能发现坏块。
func TestCheckContentSampleModeAlsoDetects(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	if _, err := st.SaveBook(ctx, book, SaveOptions{}); err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "UPDATE chapter_blocks SET packed = x'02010203'"); err != nil {
		t.Fatalf("破坏块: %v", err)
	}
	// 只有一块，抽样任意一章都会碰到它。
	rep, err := st.CheckContent(ctx, ContentCheckOptions{Sample: 2})
	if err != nil {
		t.Fatalf("CheckContent: %v", err)
	}
	if len(rep.Corrupted) != 1 {
		t.Errorf("抽样模式下损坏块数 = %d，期望 1", len(rep.Corrupted))
	}
}

// TestLoadBlockErrorCarriesLocation GetChapter 读坏块时的错误必须带 book_id 与 seq。
func TestLoadBlockErrorCarriesLocation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "UPDATE chapter_blocks SET packed = x'02010203'"); err != nil {
		t.Fatalf("破坏块: %v", err)
	}

	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	_, err = st.GetChapter(ctx, res.BookID, page.Items[0].ID)
	if err == nil {
		t.Fatal("读坏块应返回错误")
	}
	if !errors.Is(err, ErrBlockCorrupted) {
		t.Errorf("错误应可 errors.Is 到 ErrBlockCorrupted: %v", err)
	}
	var be *BlockError
	if !errors.As(err, &be) {
		t.Fatalf("错误应为 *BlockError: %v", err)
	}
	assertBlockErrorLocates(t, be, 0, 2)
	if be.BookID != res.BookID {
		t.Errorf("BookID = %d，期望 %d", be.BookID, res.BookID)
	}
	if be.ChapterID != page.Items[0].ID {
		t.Errorf("ChapterID = %d，期望 %d（触发读取的那一章）", be.ChapterID, page.Items[0].ID)
	}
}

// TestListChaptersUnaffectedByCorruptBlock 坏块不能让整本书打不开。
//
// 目录与详情只读整数列，不该碰 packed；只有真正读正文的那一章才报错。
func TestListChaptersUnaffectedByCorruptBlock(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, "UPDATE chapter_blocks SET packed = x'02'"); err != nil {
		t.Fatalf("破坏块: %v", err)
	}
	page, err := st.ListChapters(ctx, ChapterQuery{BookID: res.BookID, Limit: 10})
	if err != nil {
		t.Fatalf("坏块不应影响目录查询: %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("目录条数 = %d，期望 3", len(page.Items))
	}
	detail, err := st.GetBook(ctx, res.BookID)
	if err != nil {
		t.Fatalf("坏块不应影响书籍详情: %v", err)
	}
	if detail.ChapterCount != 3 {
		t.Errorf("详情章节数 = %d", detail.ChapterCount)
	}
}

// TestBookContentDigestStable 正文指纹必须稳定，且对内容变化敏感。
func TestBookContentDigestStable(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	book := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	res, err := st.SaveBook(ctx, book, SaveOptions{})
	if err != nil {
		t.Fatalf("SaveBook: %v", err)
	}

	first, err := st.BookContentDigest(ctx, res.BookID)
	if err != nil {
		t.Fatalf("BookContentDigest: %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := st.BookContentDigest(ctx, res.BookID)
		if err != nil {
			t.Fatalf("BookContentDigest #%d: %v", i, err)
		}
		if again != first {
			t.Fatalf("同一本书的正文指纹不稳定：%d vs %d", first, again)
		}
	}

	changed := sampleBook("长夜书简", "青川客", "/path/to/txt/a.txt", repeat64("a"))
	changed.Chapters[0].Content = "华夏国临川市，午后的春日，微风拂过城楼。"
	if _, err := st.SaveBook(ctx, changed, SaveOptions{Force: true}); err != nil {
		t.Fatalf("Force SaveBook: %v", err)
	}
	after, err := st.BookContentDigest(ctx, res.BookID)
	if err != nil {
		t.Fatalf("BookContentDigest: %v", err)
	}
	if after == first {
		t.Error("正文改动后指纹未变——它无法作为内容一致的证据")
	}
}

// TestChapterAtOffsetMatchesJoin 校验写读两侧的偏移口径完全一致。
func TestChapterAtOffsetMatchesJoin(t *testing.T) {
	contents := []string{"第一章正文", "", "第三章，含标点！", strings.Repeat("长夜书简。", 200)}
	raw, offsets := joinChapters(contents)

	for i, want := range contents {
		got, err := chapterAt(raw, offsets[i], len(want))
		if err != nil {
			t.Fatalf("第 %d 章: %v", i, err)
		}
		if got != want {
			t.Errorf("第 %d 章切片与原文不一致（%d vs %d 字节）", i, len(got), len(want))
		}
	}
}
