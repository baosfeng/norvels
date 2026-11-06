package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// 0025 的全部内容：书级健康计数（缺简介 / 缺作者 / 超薄书 / 章节数六段区间）的预计算结构。
//
// 迁移只建结构、不做回填（生产升级期间不能停监听）：三张表由 0025_book_health_counts.sql
// 建好，本步骤补两件 SQL 文件做不了的事：
//  1. books 的生成列 intro_blank + 索引——让「缺简介」的实时 SQL 也变成索引查找
//     （降级路径与明细下钻都受益）；ALTER TABLE ADD COLUMN 没有 IF NOT EXISTS，只能在 Go 里探测。
//  2. 维护触发器——计数正确性靠 SQL 触发器在所有写路径上自动维持，不需要在导入 / 追更 /
//     合并 / 拆分 / 删书 / 作者增删的 Go 代码里各补一行，也就不会出现「新写路径忘了维护」的漂移。
//
// 回填由启动后的后台任务做（books 一遍扫描），完成前读取侧回退到实时 SQL（数字仍正确）。
// 幂等：列/索引/触发器全部先探测或 IF NOT EXISTS，状态行只在缺失时插入。
func createBookHealthCountStep(ctx context.Context, tx *sql.Tx) error {
	ok, err := booksReadyForHealthCounts(ctx, tx)
	if err != nil || !ok {
		return err // 老库升级路径：books 可能不存在，或还没有 intro / author_id / chapter_count 列
	}
	if err := addBookIntroBlankColumn(ctx, tx); err != nil {
		return err
	}
	for _, ddl := range bookHealthTriggerDDL() {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("store: create book health trigger: %w", err)
		}
	}
	if err := initBookHealthState(ctx, tx); err != nil {
		return err
	}
	log.Printf("迁移 0025：已建书级健康计数表与维护触发器（历史值由后台任务回填，不阻塞监听）")
	return nil
}

// booksReadyForHealthCounts 判断 books 是否具备本迁移要用的三列（intro / author_id / chapter_count）。
//
// v1 老库的 books 只有 title 等少数列：这时既建不了生成列（表达式引用不存在的 intro），
// 建了触发器也没有可维护的列。跳过整步，读取侧因为状态行缺失/未完成而走实时 SQL——
// 那些库本来也没有这几个指标可言（它们连章节表都还没有）。
func booksReadyForHealthCounts(ctx context.Context, tx *sql.Tx) (bool, error) {
	for _, col := range []string{"intro", "author_id", "chapter_count"} {
		has, err := columnExistsInTx(ctx, tx, "books", col)
		if err != nil || !has {
			return false, err
		}
	}
	return true, nil
}

// addBookIntroBlankColumn 加生成列 intro_blank 与其索引（幂等）。
//
// 为什么用生成列而不是「索引 TRIM(intro)」：表达式索引会把整段简介正文抄进索引
// （生产 169k 本 × 数百字节 ≈ 上百 MB，每次导入都要写），而生成的布尔列索引只有 169k 个
// 小整数（几 MB）。两者都能让 TRIM(intro) = ” 不再扫全表，后者写入代价小两个量级；
// 语义完全一致：生成列的表达式就是 TRIM(intro) = ”（NULL 结果同样不计入）。
func addBookIntroBlankColumn(ctx context.Context, tx *sql.Tx) error {
	// 不用 addColumnIfMissing：它的 pragma_table_info 探测在「重放已应用过 0025 的库」时
	// 看不到虚拟生成列（modernc 的 pragma 表函数对 VIRTUAL 列的可见性），会撞 duplicate column。
	// 这里直接 ALTER，只把「列已存在」当成幂等成功。
	if _, err := tx.ExecContext(ctx,
		"ALTER TABLE books ADD COLUMN intro_blank INTEGER GENERATED ALWAYS AS (TRIM(intro) = '') VIRTUAL"); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("add generated column books.intro_blank: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"CREATE INDEX IF NOT EXISTS idx_books_intro_blank ON books(intro_blank)"); err != nil {
		return fmt.Errorf("store: create index idx_books_intro_blank: %w", err)
	}
	return nil
}

// initBookHealthState 初始化回填状态行（只在缺失时插入）。
//
// 空库（一本都没有）直接置 complete=1：之后所有写入都由触发器维持，不存在「历史待补」。
func initBookHealthState(ctx context.Context, tx *sql.Tx) error {
	var books int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&books); err != nil {
		return fmt.Errorf("store: count books for health state: %w", err)
	}
	complete := 0
	if books == 0 {
		complete = 1
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO book_health_state(id, complete, updated_at) VALUES (1, ?, ?)",
		complete, formatTime(nowUTC())); err != nil {
		return fmt.Errorf("store: init book_health_state: %w", err)
	}
	return nil
}

// bookHealthKinds 是三个标量计数的 kind（与迁移 SQL 里的 CHECK 约束一致）。
var bookHealthKinds = []string{"missing_intro", "missing_author", "thin_books"}

// introHit / authorHit / thinHit 生成「这一行算不算命中」的 SQL 表达式。
//
// 三处口径必须与实时 SQL 逐字一致（health_issues.go 的 whereMissingIntro / whereMissingAuthor、
// health.go 的 thinBookWhere）——报告走预计算、明细走实时 SQL，两条路径的差就是漂移。
func introHit(col string) string {
	return "CASE WHEN TRIM(" + col + ".intro) = '' THEN 1 ELSE 0 END"
}

// authorHit 是「缺作者」的判定：author_id = 0，或指向一个不存在的作者。
func authorHit(col string) string {
	return "CASE WHEN " + col + ".author_id = 0 OR NOT EXISTS " +
		"(SELECT 1 FROM authors a WHERE a.id = " + col + ".author_id) THEN 1 ELSE 0 END"
}

// thinHit 是「超薄书」的判定（章节数 ≤ thinBookChapters）。
func thinHit(col string) string {
	return "CASE WHEN " + col + ".chapter_count <= 1 THEN 1 ELSE 0 END"
}

// bucketIndexExpr 返回「按 chapterBucketEdges 把章节数映射成桶号」的 SQL 表达式。
//
// 口径与 bucketRangeSQL 逐字一致：第 i 桶 = [edges[i], edges[i+1])，最后一桶 [edges[last], ∞)；
// 章节数 < 1（没有章节的畸形行）不属于任何桶，返回 -1（WHERE bucket = -1 自然匹配不到）。
func bucketIndexExpr(col string) string {
	return "CASE WHEN " + col + ".chapter_count >= 1 THEN (" +
		bucketCaseExpr(col+".chapter_count", chapterBucketEdges) + ") ELSE -1 END"
}

// bookHealthTriggerDDL 生成全部维护触发器（全部 IF NOT EXISTS，重跑幂等）。
//
// 覆盖六个写入面：books 的 INSERT / 改简介 / 改作者 / 改章节数 / DELETE，
// 以及 authors 的 INSERT / DELETE（作者一出现或消失，「缺作者」的判定就变了）。
func bookHealthTriggerDDL() []string {
	insert := scalarDelta(map[string]string{
		"missing_intro":  introHit("NEW"),
		"missing_author": authorHit("NEW"),
		"thin_books":     thinHit("NEW"),
	})
	remove := scalarDelta(map[string]string{
		"missing_intro":  "-(" + introHit("OLD") + ")",
		"missing_author": "-(" + authorHit("OLD") + ")",
		"thin_books":     "-(" + thinHit("OLD") + ")",
	})
	return []string{
		"CREATE TRIGGER IF NOT EXISTS books_health_ai AFTER INSERT ON books BEGIN " +
			insert + "; " +
			"UPDATE book_chapter_buckets SET n = n + 1 WHERE bucket = " + bucketIndexExpr("NEW") + "; END",
		"CREATE TRIGGER IF NOT EXISTS books_health_ad AFTER DELETE ON books BEGIN " +
			remove + "; " +
			"UPDATE book_chapter_buckets SET n = n - 1 WHERE bucket = " + bucketIndexExpr("OLD") + "; END",
		"CREATE TRIGGER IF NOT EXISTS books_health_au_intro AFTER UPDATE OF intro ON books BEGIN " +
			"UPDATE book_health_counts SET n = n + (" + introHit("NEW") + ") - (" + introHit("OLD") + ") " +
			"WHERE kind = 'missing_intro'; END",
		"CREATE TRIGGER IF NOT EXISTS books_health_au_author AFTER UPDATE OF author_id ON books BEGIN " +
			"UPDATE book_health_counts SET n = n + (" + authorHit("NEW") + ") - (" + authorHit("OLD") + ") " +
			"WHERE kind = 'missing_author'; END",
		"CREATE TRIGGER IF NOT EXISTS books_health_au_chapters AFTER UPDATE OF chapter_count ON books BEGIN " +
			"UPDATE book_health_counts SET n = n + (" + thinHit("NEW") + ") - (" + thinHit("OLD") + ") " +
			"WHERE kind = 'thin_books'; " +
			"UPDATE book_chapter_buckets SET n = n - 1 WHERE bucket = " + bucketIndexExpr("OLD") + "; " +
			"UPDATE book_chapter_buckets SET n = n + 1 WHERE bucket = " + bucketIndexExpr("NEW") + "; END",
		authorsHealthTrigger("ai", "AFTER INSERT ON authors", "n - ", "NEW"),
		authorsHealthTrigger("ad", "AFTER DELETE ON authors", "n + ", "OLD"),
	}
}

// scalarDelta 生成「三行标量计数一起加同一个增量」的 UPDATE 语句。
//
// 一条语句改三行：导入 169k 本时每本只多三次行更新，代价可忽略。
func scalarDelta(exprs map[string]string) string {
	cases := ""
	for _, kind := range bookHealthKinds {
		cases += fmt.Sprintf(" WHEN '%s' THEN (%s)", kind, exprs[kind])
	}
	return "UPDATE book_health_counts SET n = n + (CASE kind" + cases + " ELSE 0 END)"
}

// authorsHealthTrigger 生成作者增删触发器：作者出现 → 该 id 下的书不再算「缺作者」，反之加回。
//
// op 传 "n - " / "n + "，row 传 "NEW" / "OLD"：子查询按书计数的口径与 authorHit 完全一致。
func authorsHealthTrigger(suffix, event, op, row string) string {
	return "CREATE TRIGGER IF NOT EXISTS authors_health_" + suffix + " " + event + " BEGIN " +
		"UPDATE book_health_counts SET n = " + op + "(SELECT COUNT(*) FROM books b WHERE b.author_id = " +
		row + ".id) WHERE kind = 'missing_author'; END"
}
