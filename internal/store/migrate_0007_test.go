package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// 本文件锁死 0007「正文改为按块压缩存储」的升级路径。
//
// 这是整个改动里唯一能证明「迁移没损坏正文」的证据：
// 造一个旧结构库（content 原文 + 分词镜像 + FTS），跑 0006 + 0007，
// 然后**逐章**比对迁移前后的正文，必须逐字节一致。

// seedBefore0007 建一个停在版本 6、正文仍是原文 TEXT 的库（= v0.1.0 加 0006）。
func seedBefore0007(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, _ := openBare(t, "v6.db")
	if _, err := st.MigrateTo(ctx, 6); err != nil {
		t.Fatalf("迁到版本 6: %v", err)
	}
	insertLegacyBook(t, ctx, st)
	insertPreBlockChapters(t, ctx, st)
	return st, ctx
}

// insertPreBlockChapters 按版本 6 的结构写章节（title + content，无块寻址列）。
//
// 数据刻意做得多样：空章、超长章、私用区字符、含换行与制表符、恰好跨块边界的量。
func insertPreBlockChapters(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	seed := blockMigrationSeed()
	for i, c := range seed {
		if _, err := st.DB().ExecContext(ctx,
			"INSERT INTO chapters(book_id, volume_id, idx, title, content, word_count) VALUES (1, 0, ?, ?, ?, ?)",
			i, c.title, c.content, len([]rune(c.content))); err != nil {
			t.Fatalf("插入章节 %d: %v", i, err)
		}
	}
}

// blockMigrationSeed 是 0007 迁移测试的章节种子。
//
// 覆盖：空章（验证不为空块建行）、普通章、私用区字符（验证不转码）、
// 超长章（超过 1 MB，验证独占一块）、以及足够多的普通章（触发多块切分）。
//
// 正文用**多样化**的中文句子拼装（而不是同一句重复 N 遍）：这样压缩率会落在
// 真实中文小说的量级（约 30~40%），测试顺带就能验证「压缩确实生效」；
// 同一句重复只会得到 0.0x% 的假压缩率，反而掩盖问题。
func blockMigrationSeed() []struct{ title, content string } {
	out := []struct{ title, content string }{
		{"第一章 风起", "长夜书简，灯下无人。他推开窗，风从青川吹来。"},
		{"第二章 空卷", ""},
		{"第三章 私用区", "反爬站爱用的私用区字符：\ue000\ue123\ue4ff，迁移后必须原样保留。"},
		{"第四章 控制符", "第一行\n\t第二行\r\n第三行"},
		{"第五章 长卷", variedProse(12000)},
	}
	// 再补 40 章普通内容，确保触发「按 1 MB 切多块」。
	for i := 0; i < 40; i++ {
		out = append(out, struct{ title, content string }{
			title:   fmt.Sprintf("第%d章 后续", i+6),
			content: variedProse(150),
		})
	}
	return out
}

// prosePool 是一组虚构的散文片段，用于拼装「看起来像真实小说」的测试正文。
var prosePool = []string{
	"长夜书简摊在灯下，青川剑砚压着纸角。",
	"窗外的风穿过廊下，铜铃一声接一声地响。",
	"他提笔写下第一行字，墨迹在纸上慢慢洇开。",
	"像是许多年前的雪，落在没人走过的山道上。",
	"荒渡的灯影在雾里晃，船夫始终没有说话。",
	"远处的钟声一下一下，敲得人心里发空。",
	"旧札残篇里只剩三个字，剩下的都叫蛀虫吃了。",
	"芜城的春天来得晚，柳絮落到四月才肯飞。",
	"他把剑横在膝上，看着火堆一点点矮下去。",
	"孤灯旅舍的掌柜打着哈欠，说前头山路不太平。",
	"雨下了一夜，青石巷里全是水声。",
	"雪停之后，山脊像一条搁浅的白鱼。",
	"茶凉了，他也没起身去续。",
	"那句话他记了很多年，却始终没想明白是什么意思。",
	"马蹄声近了，又远了。",
	"她在廊下站着，手里那盏灯一直没灭。",
}

// variedProse 用 prosePool 拼出 n 段正文，句子后面再接一段**高熵**汉字序列。
//
// 为什么混入高熵部分：如果整段都是同一批句子循环，压缩率会虚高到 0.1%，
// 测试就失去「验证压缩真的生效」的意义。真实小说是「高频词 + 大段描述」的混合，
// 这里用确定性伪随机汉字模拟后者，让压缩率落在可信区间（约 30~60%）。
func variedProse(n int) string {
	var b strings.Builder
	b.Grow(n * 40)
	seed := uint64(n*2654435761 + 1)
	for i := 0; i < n; i++ {
		b.WriteString(prosePool[(i*7+i/3)%len(prosePool)])
		if i%6 == 5 {
			b.WriteString("\n")
		}
		// 每 3 句补一段伪随机汉字（确定性：同一个 n 得到同一段文本）。
		// 用 splitmix64 混合而不是朴素 LCG：LCG 的高位有可被压缩器学到的结构，
		// 会把压缩率虚报成 13%，掩盖「压缩器没学到东西」这类真实问题。
		// 每句补一段高熵汉字，让「均匀随机字」占比接近一半——这与真实小说
		// 在压缩器眼里的可预测性更接近，压缩率才会落在 30%~40% 的可信区间。
		for j := 0; j < 14; j++ {
			seed += 0x9E3779B97F4A7C15
			z := seed
			z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
			z = (z ^ (z >> 27)) * 0x94D049BB133111EB
			z ^= z >> 31
			b.WriteRune(rune(0x4E00 + z%0x5000))
		}
		b.WriteString("。")
	}
	return b.String()
}

// chapterDigestsByID 返回「章节 id → 正文 SHA256」表，且要求指纹覆盖到每一章。
//
// 用 SHA256 而不是直接存全文字符串：种子含 1 MB 级的长章，
// 把几十章正文全塞进测试内存没有意义，指纹同样能证明逐字节一致。
func chapterDigestsByID(t *testing.T, ctx context.Context, st *Store) map[int64]string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT id, content FROM chapters ORDER BY id")
	if err != nil {
		t.Fatalf("读正文: %v", err)
	}
	defer rows.Close()

	out := make(map[int64]string, 64)
	for rows.Next() {
		var (
			id      int64
			content string
		)
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatalf("扫描正文: %v", err)
		}
		sum := sha256.Sum256([]byte(content))
		out[id] = hex.EncodeToString(sum[:])
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历正文: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("指纹表为空，断言会退化为恒真")
	}
	return out
}

// readChapterContentsByID 升级后逐章解压，返回 id → 正文。
func readChapterContentsByID(t *testing.T, ctx context.Context, st *Store) map[int64]string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, "SELECT id FROM chapters ORDER BY id")
	if err != nil {
		t.Fatalf("列章节: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("扫描 id: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 id: %v", err)
	}

	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		ch, err := st.GetChapter(ctx, 0, id)
		if err != nil {
			t.Fatalf("GetChapter(%d): %v", id, err)
		}
		out[id] = ch.Content
	}
	return out
}

// TestMigrate0007KeepsContentByteIdentical 是本次改动最关键的断言。
func TestMigrate0007KeepsContentByteIdentical(t *testing.T) {
	st, ctx := seedBefore0007(t)

	beforeCount := countRow(t, ctx, st, "SELECT COUNT(*) FROM chapters")
	beforeDigests := chapterDigestsByID(t, ctx, st)
	beforeBlocks := countRow(t, ctx, st, "SELECT COUNT(*) FROM sqlite_master WHERE name='chapter_blocks'")

	// 前置确认：造出来的是「有 content 列、无块表」的版本 6 结构。
	if beforeBlocks != 0 {
		t.Fatalf("迁移前就存在 chapter_blocks，测试前提不成立")
	}
	if !columnExists(t, ctx, st, "chapters", "content") {
		t.Fatal("迁移前 chapters 应有 content 列")
	}

	status, err := st.MigrateTo(ctx, LatestVersion())
	if err != nil {
		t.Fatalf("升级到最新版本: %v", err)
	}
	if status.FailedVersion != 0 {
		t.Errorf("迁移失败于版本 %d", status.FailedVersion)
	}

	assertBlockSchema(t, ctx, st)
	assertCountRow(t, ctx, st, "SELECT COUNT(*) FROM chapters", beforeCount, "章节数")
	assertEveryChapterIdentical(t, ctx, st, beforeDigests)
	assertBlockCoverage(t, ctx, st)
	assertMetricsAfterBlocks(t, ctx, st)
}

// assertBlockBytesMatchMetrics 对账「块表真实字节」与「metrics 缓存值」。
//
// 口径必须逐字节相等：增量累加（导入时）与全量重算（RefreshMetrics）是两条代码路径，
// 一旦漂移，/api/stats 显示的正文字节就与实际占用对不上。
func assertBlockBytesMatchMetrics(t *testing.T, ctx context.Context, st *Store) (packed, raw int64) {
	t.Helper()
	packed = scalarInt(t, ctx, st, "SELECT COALESCE(SUM(length(packed)),0) FROM chapter_blocks")
	raw = scalarInt(t, ctx, st, "SELECT COALESCE(SUM(raw_bytes),0) FROM chapter_blocks")
	if packed <= 0 || raw <= 0 {
		t.Fatalf("块表口径异常：packed=%d raw=%d", packed, raw)
	}
	if packed >= raw {
		t.Errorf("packed(%d) 未小于 raw(%d)——压缩没生效", packed, raw)
	}
	if got := scalarInt(t, ctx, st, "SELECT value FROM store_metrics WHERE key = 'text_bytes'"); got != packed {
		t.Errorf("metrics text_bytes = %d，块表 SUM(length(packed)) = %d（口径漂移）", got, packed)
	}
	if got := scalarInt(t, ctx, st, "SELECT value FROM store_metrics WHERE key = 'text_bytes_raw'"); got != raw {
		t.Errorf("metrics text_bytes_raw = %d，块表 SUM(raw_bytes) = %d（口径漂移）", got, raw)
	}
	return packed, raw
}

// scalarInt 执行单值整数查询。
func scalarInt(t *testing.T, ctx context.Context, st *Store, query string) int64 {
	t.Helper()
	var v int64
	if err := st.DB().QueryRowContext(ctx, query).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

// countRow 执行单值计数查询。
func countRow(t *testing.T, ctx context.Context, st *Store, query string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRowContext(ctx, query).Scan(&n); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return n
}

// assertCountRow 断言计数不变。
func assertCountRow(t *testing.T, ctx context.Context, st *Store, query string, want int, label string) {
	t.Helper()
	got := countRow(t, ctx, st, query)
	if got != want {
		t.Errorf("%s：迁移后 %d，迁移前 %d", label, got, want)
	}
}

// assertBlockSchema 校验升级后的表结构。
func assertBlockSchema(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if columnExists(t, ctx, st, "chapters", "content") {
		t.Errorf("chapters 仍有 content 列（正文应已搬进块）")
	}
	for _, col := range []string{"block_id", "block_off", "block_len"} {
		if !columnExists(t, ctx, st, "chapters", col) {
			t.Errorf("chapters 缺少块寻址列 %q", col)
		}
	}
	for _, col := range []string{"book_id", "seq", "first_idx", "last_idx", "raw_bytes", "packed"} {
		if !columnExists(t, ctx, st, "chapter_blocks", col) {
			t.Errorf("chapter_blocks 缺少列 %q", col)
		}
	}
	if exists, err := st.tableExists(ctx, "chapter_blocks"); err != nil || !exists {
		t.Fatalf("chapter_blocks 表不存在（err=%v）", err)
	}
}

// assertEveryChapterIdentical 逐章比对迁移前后的正文指纹。
func assertEveryChapterIdentical(t *testing.T, ctx context.Context, st *Store, before map[int64]string) {
	t.Helper()
	after := readChapterContentsByID(t, ctx, st)
	if len(after) != len(before) {
		t.Fatalf("章节数不一致：迁移前 %d，迁移后 %d", len(before), len(after))
	}
	checked := 0
	for id, wantDigest := range before {
		content, ok := after[id]
		if !ok {
			t.Errorf("章节 %d 在迁移后消失", id)
			continue
		}
		sum := sha256.Sum256([]byte(content))
		if got := hex.EncodeToString(sum[:]); got != wantDigest {
			t.Errorf("章节 %d 正文变了：迁移前 %s，迁移后 %s（%d 字节）",
				id, wantDigest, got, len(content))
			continue
		}
		checked++
	}
	if checked != len(before) {
		t.Errorf("只有 %d/%d 章校验通过", checked, len(before))
	}
	t.Logf("逐章比对：%d 章全部逐字节一致", checked)
}

// assertBlockCoverage 校验每章都被块覆盖（或确实是空章）。
func assertBlockCoverage(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	var orphan int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapters WHERE block_id = 0 AND block_len <> 0").Scan(&orphan); err != nil {
		t.Fatalf("查孤立章节: %v", err)
	}
	if orphan != 0 {
		t.Errorf("%d 章有长度但没有块归属", orphan)
	}
	var badSpan int
	if err := st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM chapters c "+
		"JOIN chapter_blocks b ON b.id = c.block_id "+
		"WHERE c.block_off < 0 OR c.block_off + c.block_len > b.raw_bytes").Scan(&badSpan); err != nil {
		t.Fatalf("查越界章节: %v", err)
	}
	if badSpan != 0 {
		t.Errorf("%d 章的块内区间越界", badSpan)
	}
	var badRange int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chapter_blocks WHERE first_idx > last_idx").Scan(&badRange); err != nil {
		t.Fatalf("查块区间: %v", err)
	}
	if badRange != 0 {
		t.Errorf("%d 个块的 first_idx > last_idx", badRange)
	}
}

// assertMetricsAfterBlocks 校验体积口径已切到 packed 字节。
func assertMetricsAfterBlocks(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()
	if err := st.RefreshMetrics(ctx); err != nil {
		t.Fatalf("RefreshMetrics: %v", err)
	}
	snap, err := st.Metrics(ctx)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if snap.Stale {
		t.Errorf("重算后不应 stale")
	}
	if snap.TextBytes <= 0 {
		t.Errorf("TextBytes = %d，压缩后应有正值", snap.TextBytes)
	}
	if snap.TextBytesRaw <= 0 {
		t.Errorf("TextBytesRaw = %d，原文逻辑字节应有正值", snap.TextBytesRaw)
	}
	packed, raw := assertBlockBytesMatchMetrics(t, ctx, st)
	t.Logf("正文口径：packed=%d 字节，原文=%d 字节，压缩率 %.1f%%",
		packed, raw, 100.0*float64(packed)/float64(raw))
}
