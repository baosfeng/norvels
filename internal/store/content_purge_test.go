package store

import (
	"context"
	"strings"
	"testing"
)

// 删书后的正文库清理（purgeBookContent / purgeBookFingerprints）是 best-effort：
// 书已经从书架消失，正文库清理失败只记日志、不让删除失败。这里断言
// 「表在的时候真的清干净」与「老库 / 非法 id 静默跳过」两条。

func TestPurgeBookContentClearsBlocksAndFingerprints(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	res := saveOrFail(t, ctx, st, syntheticBook("清理之书", "/books/purge.txt",
		strings.Repeat("e", 64), syntheticChapters(3)))
	st.purgeBookContent(ctx, res.BookID)

	requireEqual(t, "剩余正文块", scalarIntBlock(t, ctx, st,
		"SELECT COUNT(*) FROM chapter_blocks WHERE book_id = ?", res.BookID), int64(0))
	requireEqual(t, "剩余指纹行", fingerprintRows(t, ctx, st, res.BookID), 0)
	// 覆盖行必须同事务清掉：留着会把一本已删的书算成「已覆盖」，判重缺口就看不见了。
	assertCoverageMatchesFull(t, ctx, st)
}

func TestPurgeBookContentGuards(t *testing.T) {
	ctx := context.Background()
	// 非正数 id（调用方用 0 表示「这本书没有块」）：直接返回，不查库也不报错。
	fresh := newTestStore(t)
	fresh.purgeBookContent(ctx, 0)
	fresh.purgeBookFingerprints(ctx, -1)

	// 0017：正文库还没有 chapter_blocks；0018：有块表但还没有 0019 的指纹表。
	// 两种老库都必须静默跳过（块还在主库，由 books 的级联顺手删掉）。
	newTestStoreTo(t, 17).purgeBookContent(ctx, 1)
	newTestStoreTo(t, 18).purgeBookContent(ctx, 1)
}
