package recstore

import (
	"context"
	"fmt"
)

// recSchemaVersion 是推荐库的表结构版本（PRAGMA user_version）。
//
// 为什么不用主库的迁移体系：本库整库可重建（设计 §1），加一列可以「删文件重来」，
// 不需要 20 分钟不可逆的前向迁移；用 user_version 只为挡住「新库配旧程序」这种
// 会读到错列的组合。
// 版本历史：1 = 初始结构；2 = 负反馈表 negative_items（dislike 用）。
// 升版只做「幂等建表」（所有 DDL 都是 IF NOT EXISTS），不做数据迁移。
const recSchemaVersion = 2

// recSchemaDDL 是推荐库的全部结构，只在这里声明一次。
//
// WITHOUT ROWID 的取舍：user_interest / cooccur / recent_items / book_daily 的访问
// 全部走主键前缀（按用户查兴趣、按书查共读、按用户+书查曝光），聚簇在主键上就
// 少一次 rowid 回表、也少养一份主键索引；book_stats 用 INTEGER 主键（rowid 别名），
// 它按 book_id 单点更新，没必要另建聚簇。
var recSchemaDDL = []string{
	// 只落「有信号」的事件（impression 不进这张表，见 RecordEvents）。
	`CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    kind        TEXT    NOT NULL,
    book_id     INTEGER NOT NULL DEFAULT 0,
    author_id   INTEGER NOT NULL DEFAULT 0,
    category_l1 TEXT    NOT NULL DEFAULT '',
    category_l2 TEXT    NOT NULL DEFAULT '',
    value       INTEGER NOT NULL DEFAULT 0,
    extra       TEXT    NOT NULL DEFAULT '',
    ts          TEXT    NOT NULL
)`,
	"CREATE INDEX IF NOT EXISTS idx_events_user_kind ON events(user_id, kind, ts)",
	"CREATE INDEX IF NOT EXISTS idx_events_book ON events(book_id, kind)",
	`CREATE TABLE IF NOT EXISTS book_stats (
    book_id      INTEGER PRIMARY KEY,
    impressions  INTEGER NOT NULL DEFAULT 0,
    clicks       INTEGER NOT NULL DEFAULT 0,
    reads        INTEGER NOT NULL DEFAULT 0,
    read_seconds INTEGER NOT NULL DEFAULT 0,
    shelf_adds   INTEGER NOT NULL DEFAULT 0,
    last_at      TEXT    NOT NULL DEFAULT ''
)`,
	// 按日聚合：热度窗口与「最近很多人读」的排序都读这张表，不扫 events 明细。
	`CREATE TABLE IF NOT EXISTS book_daily (
    book_id     INTEGER NOT NULL,
    day         TEXT    NOT NULL,
    impressions INTEGER NOT NULL DEFAULT 0,
    clicks      INTEGER NOT NULL DEFAULT 0,
    reads       INTEGER NOT NULL DEFAULT 0,
    seconds     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, day)
) WITHOUT ROWID`,
	// 主键是 (book_id, day)，按日筛窗口（近 7 天）用不上主键前缀，单独建 day 索引。
	"CREATE INDEX IF NOT EXISTS idx_book_daily_day ON book_daily(day, book_id)",
	// 兴趣权重：dim 只有 category / author 两种，key 是分类名或作者 id 的十进制文本。
	`CREATE TABLE IF NOT EXISTS user_interest (
    user_id    INTEGER NOT NULL,
    dim        TEXT    NOT NULL,
    key        TEXT    NOT NULL,
    weight     REAL    NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL,
    PRIMARY KEY (user_id, dim, key)
) WITHOUT ROWID`,
	// 共读：双向各存一行（A→B 与 B→A），查询时不必写 OR，代价是两倍行数。
	`CREATE TABLE IF NOT EXISTS cooccur (
    book_id  INTEGER NOT NULL,
    other_id INTEGER NOT NULL,
    users    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, other_id)
) WITHOUT ROWID`,
	// 近期已曝光：曝光量最大，只按 (user_id, book_id) 去重保留最近一次时间，
	// 不保留曝光次数——次数在 book_stats / book_daily 里已经聚合过了。
	`CREATE TABLE IF NOT EXISTS recent_items (
    user_id INTEGER NOT NULL,
    book_id INTEGER NOT NULL,
    ts      TEXT    NOT NULL,
    PRIMARY KEY (user_id, book_id)
) WITHOUT ROWID`,
	// 负反馈（dislike）：按 (user, book) 去重只留最近一次时间，召回阶段整本剔除。
	// 与 recent_items 同样 WITHOUT ROWID：查询永远是「某账号的整集合」。
	`CREATE TABLE IF NOT EXISTS negative_items (
    user_id INTEGER NOT NULL,
    book_id INTEGER NOT NULL,
    ts      TEXT    NOT NULL,
    PRIMARY KEY (user_id, book_id)
) WITHOUT ROWID`,
}

// ensureSchema 幂等建立推荐库结构，并用 user_version 标记版本。
//
// 版本高于程序时**拒绝打开**而不是带病运行：新库可能有旧程序读不懂的列，
// 继续跑只会在第一次查询时抛「no such column」，把配置问题伪装成运行期故障。
func (s *Store) ensureSchema(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("recstore: 读 schema 版本: %w", err)
	}
	if version > recSchemaVersion {
		return fmt.Errorf("recstore: 库版本 %d 高于程序支持的 %d（用新版 norvels 打开，或删库重建）",
			version, recSchemaVersion)
	}
	if version == recSchemaVersion {
		return nil
	}
	return s.createSchema(ctx)
}

// createSchema 在一个事务里建表并写版本号（建表失败不会留下「半套表 + 版本 0」）。
func (s *Store) createSchema(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recstore: 建表事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, ddl := range recSchemaDDL {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("recstore: 建表: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", recSchemaVersion)); err != nil {
		return fmt.Errorf("recstore: 写 schema 版本: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("recstore: 提交 schema: %w", err)
	}
	return nil
}
