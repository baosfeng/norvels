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

// TestImportVersionPickUpgradeCandidate 旧版脏、新版干净：第二本判「升级候选」，且不自动替换。
func TestImportVersionPickUpgradeCandidate(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	oldID, oldQuality := importDirtyVersion(t, ctx, st)

	res, files := importDedupe(t, ctx, st, "新版.txt", versionNovel("甲书", false, 0), false)
	if res.Skipped != 1 || len(res.Imported) != 0 {
		t.Fatalf("新版导入汇总 = %+v，期望按升级候选跳过（不入库）", res)
	}
	if len(res.UpgradeCandidates) != 1 {
		t.Fatalf("升级候选 = %d 条，期望 1 条", len(res.UpgradeCandidates))
	}
	assertUpgradeCandidate(t, res.UpgradeCandidates[0], oldID, oldQuality)
	assertUpgradeReason(t, onlyJobFile(t, files))
	assertNotReplaced(t, ctx, st, oldID, oldQuality)
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

// assertUpgradeReason 断言文件终态与备注（跳过 + 升级候选 + 需人工确认）。
func assertUpgradeReason(t *testing.T, f store.ImportJobFile) {
	t.Helper()
	assertFileState(t, f, store.ImportFileStatusSkipped, "升级候选")
	for _, want := range []string{"《旧版》", "噪声率", "乱码率", "需人工确认替换"} {
		if !strings.Contains(f.Reason, want) {
			t.Errorf("备注 = %q，期望包含 %q", f.Reason, want)
		}
	}
}

// assertNotReplaced 断言「不自动替换」：库内仍只有一本书，旧版质量分没被改写。
func assertNotReplaced(t *testing.T, ctx context.Context, st *store.Store, oldID int64, oldQuality store.BookQuality) {
	t.Helper()
	if n := bookTotal(t, ctx, st); n != 1 {
		t.Errorf("库内书数 = %d，期望 1（升级候选不入库、不替换）", n)
	}
	if after := mustQuality(t, ctx, st, oldID); after != oldQuality {
		t.Errorf("旧版质量分被改写了：%+v → %+v", oldQuality, after)
	}
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
