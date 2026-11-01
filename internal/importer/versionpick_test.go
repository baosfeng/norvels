package importer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"norvels/internal/store"
)

// 本文件是「版本择优」（设计 §5）在导入链路上的端到端证据：两本内容几乎相同的书，
// 一本脏（广告行 + 乱码）一本干净，走真实 Run 判出「升级候选」或「重复」。
//
// 为什么用真实解析链路而不是构造质量分：判定要证明的正是「从脏文件到干净文件」
// 这条路上质量分真的把差异量化了出来（噪声行被清洗器丢掉 → 噪声率、乱码字符 → 乱码率）。
// 语料全是造出来的（虚构书名 + 合成正文），不碰真实语料与真实书名。

// versionChapterCount 是版本对照用的章数：40 章里有 1 章不同，按字节加权的命中率
// 仍在 0.95 之上（留足余量，不卡在阈值边界上）。
const versionChapterCount = 40

// versionNovel 造一本合成小说：
//   - dirty 为真时每章后插一行广告（清洗器会整行丢弃，只抬噪声率、不改章节正文）；
//   - brokenChapter 为 1 基章号时那一章正文尾部多一串 U+FFFD（乱码率 > 0，
//     且该章指纹与干净版不同，命中率因此不是 100%）。
func versionNovel(title string, dirty bool, brokenChapter int) string {
	var b strings.Builder
	b.WriteString("书名：" + title + "\n作者：示例作者\n\n")
	for i := 0; i < versionChapterCount; i++ {
		fmt.Fprintf(&b, "第%d章 风雪%d\n", i+1, i)
		b.WriteString(dedupeBody(i))
		if brokenChapter == i+1 {
			b.WriteString("乱码\uFFFD\uFFFD\uFFFD\uFFFD\n")
		}
		if dirty {
			b.WriteString("本作品来自互联网，请支持正版。\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestImportVersionPickUpgradeCandidate 旧版脏、新版干净：第二本判「升级候选」。
//
// 0021 起升级候选**照常入库但立刻隐藏**（books.superseded_by 指向旧书），并落一条
// pending 审核记录：书架不会多出一本重复的书，而两版正文都在库里，替换 / 回滚都只改
// 两个整数字段。自动替换仍然不做（设计 §7）。
func TestImportVersionPickUpgradeCandidate(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	oldID, oldQuality := importDirtyVersion(t, ctx, st)

	res, files := importDedupe(t, ctx, st, "新版.txt", versionNovel("甲书", false, 0), false)
	if res.Skipped != 0 || len(res.Imported) != 1 {
		t.Fatalf("新版导入汇总 = %+v，期望 1 本入库（升级候选入库但隐藏）", res)
	}
	if len(res.UpgradeCandidates) != 1 {
		t.Fatalf("升级候选 = %d 条，期望 1 条", len(res.UpgradeCandidates))
	}
	cand := res.UpgradeCandidates[0]
	assertUpgradeCandidate(t, cand, oldID, oldQuality)
	if cand.NewBookID != res.Imported[0] || cand.ReviewID <= 0 {
		t.Fatalf("候选指向 = 新书 %d / 审核 %d，期望新书 %d 且审核记录已落",
			cand.NewBookID, cand.ReviewID, res.Imported[0])
	}
	assertUpgradeReason(t, onlyJobFile(t, files), cand.ReviewID)
	assertUpgradeReview(t, ctx, st, cand, oldID, oldQuality)
	assertNotReplaced(t, ctx, st, cand, oldID, oldQuality)
}

// assertUpgradeReview 断言审核记录的形状：pending + 命中率 + 规则对比文案。
func assertUpgradeReview(t *testing.T, ctx context.Context, st *store.Store,
	cand UpgradeCandidate, oldID int64, oldQuality store.BookQuality) {
	t.Helper()
	rev, err := st.GetImportReview(ctx, cand.ReviewID)
	if err != nil {
		t.Fatalf("读审核记录 %d: %v", cand.ReviewID, err)
	}
	if rev.Status != store.ReviewStatusPending {
		t.Errorf("审核状态 = %q，期望 pending（等 AI / 人工）", rev.Status)
	}
	if rev.OldBookID != oldID || rev.NewBookID != cand.NewBookID {
		t.Errorf("审核记录的书 = 旧 %d / 新 %d，期望 旧 %d / 新 %d",
			rev.OldBookID, rev.NewBookID, oldID, cand.NewBookID)
	}
	if rev.HitRatio <= 0 || rev.HitRatio > 1 {
		t.Errorf("命中率 = %v，期望 (0,1]", rev.HitRatio)
	}
	if len(rev.RuleReasons) < 2 {
		t.Errorf("规则对比 = %v，期望至少两项（噪声率 / 乱码率）", rev.RuleReasons)
	}
	if rev.JobID == 0 {
		t.Errorf("审核记录没有关联导入任务")
	}
	// 旧版本质量分不受影响（审核只加记录，不动旧书）。
	if after := mustQuality(t, ctx, st, oldID); after != oldQuality {
		t.Errorf("旧版质量分被改写了：%+v → %+v", oldQuality, after)
	}
}

// importDirtyVersion 导入「旧版（脏）」并返回它的 id 与质量分（脏语料必须真的脏）。
func importDirtyVersion(t *testing.T, ctx context.Context, st *store.Store) (int64, store.BookQuality) {
	t.Helper()
	res, _ := importDedupe(t, ctx, st, "旧版.txt", versionNovel("甲书", true, 7), false)
	if len(res.Imported) != 1 {
		t.Fatalf("旧版导入汇总 = %+v，期望 1 本入库", res)
	}
	id := res.Imported[0]
	q := mustQuality(t, ctx, st, id)
	if q.NoiseRatio <= 0 || q.GarbageRatio <= 0 {
		t.Fatalf("旧版质量分 = %+v，期望噪声率与乱码率都 > 0（语料要真的脏）", q)
	}
	return id, q
}

// assertUpgradeCandidate 断言候选指向旧书、至少两项更优、且新旧差异方向正确。
//
// 书名取自文件名（解析链路的标题优先级：文件名 > 正文头部元信息）。
func assertUpgradeCandidate(t *testing.T, cand UpgradeCandidate, oldID int64, oldQuality store.BookQuality) {
	t.Helper()
	if cand.OldBookID != oldID || cand.OldBookTitle != "旧版" {
		t.Errorf("候选旧书 = %d《%s》，期望 %d《旧版》", cand.OldBookID, cand.OldBookTitle, oldID)
	}
	if len(cand.Better) < 2 || len(cand.Worse) != 0 {
		t.Errorf("候选差异 = 更优 %v / 更差 %v，期望至少两项更优且零项更差", cand.Better, cand.Worse)
	}
	if cand.Old != oldQuality {
		t.Errorf("候选里的旧质量分 = %+v，期望与库内一致 %+v", cand.Old, oldQuality)
	}
	if cand.New.NoiseRatio >= cand.Old.NoiseRatio || cand.New.GarbageRatio >= cand.Old.GarbageRatio {
		t.Errorf("候选质量对比 = 旧 %+v → 新 %+v，期望噪声率与乱码率都下降", cand.Old, cand.New)
	}
}

// assertUpgradeReason 断言文件终态与备注（入库 + 隐藏 + 待审核 + 审核记录 id）。
func assertUpgradeReason(t *testing.T, f store.ImportJobFile, reviewID int64) {
	t.Helper()
	assertFileState(t, f, store.ImportFileStatusImported, "升级候选")
	for _, want := range []string{"《旧版》", "噪声率", "乱码率", "已入库并隐藏", "待审核"} {
		if !strings.Contains(f.Reason, want) {
			t.Errorf("备注 = %q，期望包含 %q", f.Reason, want)
		}
	}
	if !strings.Contains(f.Reason, fmt.Sprintf("#%d", reviewID)) {
		t.Errorf("备注 = %q，期望带上审核记录 id #%d", f.Reason, reviewID)
	}
}

// assertNotReplaced 断言「不自动替换」：书架上只有旧版，新版本被隐藏，两本书都在库里。
func assertNotReplaced(t *testing.T, ctx context.Context, st *store.Store,
	cand UpgradeCandidate, oldID int64, oldQuality store.BookQuality) {
	t.Helper()
	page, err := st.ListBooks(ctx, store.BookQuery{})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != oldID {
		t.Errorf("书架 = %d 本 %+v，期望只剩旧版 %d（新版本隐藏）", page.Total, page.Items, oldID)
	}
	if n := allBookTotal(t, ctx, st); n != 2 {
		t.Errorf("库内书数 = %d，期望 2（新版本入库但隐藏）", n)
	}
	if got := supersededBy(t, ctx, st, cand.NewBookID); got != oldID {
		t.Errorf("新版本 superseded_by = %d，期望 %d（被旧书取代 = 隐藏）", got, oldID)
	}
	if got := supersededBy(t, ctx, st, oldID); got != 0 {
		t.Errorf("旧版本 superseded_by = %d，期望 0（仍是当前版本）", got)
	}
	if after := mustQuality(t, ctx, st, oldID); after != oldQuality {
		t.Errorf("旧版质量分被改写了：%+v → %+v", oldQuality, after)
	}
}

// allBookTotal 数主库里的全部书（含被隐藏的旧版本），与「书架可见数」区分开。
func allBookTotal(t *testing.T, ctx context.Context, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&n); err != nil {
		t.Fatalf("数全部书: %v", err)
	}
	return n
}

// supersededBy 读一本书的 superseded_by（0 = 当前版本）。
func supersededBy(t *testing.T, ctx context.Context, st *store.Store, bookID int64) int64 {
	t.Helper()
	var v int64
	if err := st.DB().QueryRowContext(ctx, "SELECT superseded_by FROM books WHERE id = ?", bookID).Scan(&v); err != nil {
		t.Fatalf("读书 %d 的 superseded_by: %v", bookID, err)
	}
	return v
}

// TestImportVersionPickNewVersionWorse 新版更脏：判重复，备注写「质量更优」。
func TestImportVersionPickNewVersionWorse(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	cleanRes, _ := importDedupe(t, ctx, st, "干净版.txt", versionNovel("乙书", false, 0), false)
	if len(cleanRes.Imported) != 1 {
		t.Fatalf("干净版导入汇总 = %+v，期望 1 本入库", cleanRes)
	}

	res, files := importDedupe(t, ctx, st, "更脏版.txt", versionNovel("乙书", true, 5), false)
	if res.Skipped != 1 || len(res.UpgradeCandidates) != 0 {
		t.Fatalf("更脏版导入汇总 = %+v，期望判重复且没有升级候选", res)
	}
	f := onlyJobFile(t, files)
	assertFileState(t, f, store.ImportFileStatusSkipped, "疑似重复")
	if !strings.Contains(f.Reason, "质量更优") {
		t.Errorf("备注 = %q，期望说明库内版本质量更优", f.Reason)
	}
	if n := bookTotal(t, ctx, st); n != 1 {
		t.Errorf("库内书数 = %d，期望 1（重复不入库）", n)
	}
}

// mustQuality 读一本书的质量分（读不到直接失败）。
func mustQuality(t *testing.T, ctx context.Context, st *store.Store, bookID int64) store.BookQuality {
	t.Helper()
	q, err := st.BookQuality(ctx, bookID)
	if err != nil {
		t.Fatalf("读书 %d 的质量分: %v", bookID, err)
	}
	return q
}

// bookTotal 返回主库书数（升级候选不该改变它）。
func bookTotal(t *testing.T, ctx context.Context, st *store.Store) int {
	t.Helper()
	page, err := st.ListBooks(ctx, store.BookQuery{})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	return page.Total
}

// TestUpgradeNoteAndReasons 「升级候选」的备注与规则文案口径（含审核记录落不成时的降级）。
func TestUpgradeNoteAndReasons(t *testing.T) {
	cand := UpgradeCandidate{OldBookTitle: "旧版", Better: []store.QualityDimension{
		{Key: store.QualityDimNoise, Old: 0.04, New: 0},
	}}
	withID := upgradeNote(cand, 12)
	for _, want := range []string{"#12", "已入库并隐藏", "《旧版》", "噪声率"} {
		if !strings.Contains(withID, want) {
			t.Errorf("带审核 id 的备注 = %q，期望包含 %q", withID, want)
		}
	}
	if got := upgradeNote(cand, 0); !strings.Contains(got, "本轮不自动替换") {
		t.Errorf("无审核 id 的备注 = %q，期望回落到「本轮不自动替换」", got)
	}
	if got := titleOrUnknown(""); got != "书名未知" {
		t.Errorf("缺书名 = %q，期望「书名未知」", got)
	}
	reasons := dimensionReasons(cand.Better)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "噪声率") {
		t.Errorf("规则文案 = %v，期望一条噪声率", reasons)
	}
	if got := (&runState{}).jobID(); got != 0 {
		t.Errorf("无任务时 jobID = %d，期望 0", got)
	}
}

// TestRecordUpgradeFailure 审核记录落不成时不拦导入：备注说明「已按普通书入库」。
func TestRecordUpgradeFailure(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	r := &runState{st: st, res: &Result{}}
	note := r.recordUpgrade(ctx, UpgradeCandidate{OldBookID: 9999}, 1)
	if !strings.Contains(note, "审核记录创建失败") {
		t.Errorf("备注 = %q，期望说明审核记录创建失败", note)
	}
	if r.lastErr == "" {
		t.Errorf("失败原因没有记进 lastErr")
	}
	if len(r.res.UpgradeCandidates) != 0 {
		t.Errorf("失败的候选不该进汇总：%+v", r.res.UpgradeCandidates)
	}
}
