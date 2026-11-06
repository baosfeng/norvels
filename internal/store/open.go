package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"norvels/internal/observe"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（CGO-free）
)

// Store 包装**两个** SQLite 连接池：
//
//   - db：主库（books / volumes / chapters / 用户数据 / 聚合缓存），几十 MB 量级；
//   - cdb：正文库（只有 chapter_blocks），GB 量级。
//
// 为什么拆两个文件：正文占库体积 95%（见 ADR-0007），和元数据放在同一个文件里，
// 备份、VACUUM、损坏修复都得一起做，元数据侧的写放大也被正文页牵连。
// 两个库之间**没有外键、也没有跨库事务**：chapters.block_id 是弱引用，
// 一致性靠写入顺序 + 后台对账（见 docs/architecture.md §2.3、ReconcileContent）。
type Store struct {
	db   *sql.DB // 主库
	cdb  *sql.DB // 正文库（chapter_blocks）
	path string
	// contentPath 是正文库文件路径（由主库路径推导或 NORVELS_CONTENT_DB 覆盖）。
	contentPath string
	// metricsRefreshing 去重后台补算（同一时刻只允许一个 goroutine 重算）。
	metricsRefreshing atomic.Bool
	// precomputeReady 缓存「三张预计算表可用且口径正确」的探测结果。
	// 按 Store 实例持有而不是包级变量：同一进程里连着两个库（生产库 + 临时库）时，
	// 包级缓存会把「这个库有 0022」误当成「那个库也有」。
	precomputeReady atomic.Bool
	// staleCallback 在后台补算完成后调用（api 层用来记日志），可为 nil。
	staleCallback func(err error)
	// reconcile 缓存上一次正文对账结果（见 ReconcileContent）。
	reconcile reconcileCache
}

// Open 打开（不存在则创建）主库与正文库并应用 PRAGMA。
// path 为主库文件路径，例如 ./data/norvels.db；正文库默认是**同目录兄弟文件**
// （norvels.db → norvels-content.db），可用 NORVELS_CONTENT_DB 覆盖。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("store: empty database path")
	}
	contentPath := contentDBPath(path)
	for _, p := range []string{path, contentPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, fmt.Errorf("store: create dir: %w", err)
		}
	}
	db, err := openStoreSQLite(ctx, path, "main")
	if err != nil {
		return nil, err
	}
	cdb, err := openStoreSQLite(ctx, contentPath, "content")
	if err != nil {
		db.Close() //nolint:errcheck // 打开失败路径，原错误优先
		return nil, err
	}
	return &Store{db: db, cdb: cdb, path: path, contentPath: contentPath}, nil
}

// openSQLite 打开一个 SQLite 文件并应用统一 PRAGMA（两库共用同一套连接参数）。
//
// _txlock=immediate：本库的事务全部是写事务（读走 db.Query，不开事务），
// 显式起写事务让并发的第二个写者老实等 busy_timeout，而不是拿着旧读快照
// 在第一条写语句上直接吃 SQLITE_BUSY——WAL 下「快照已过期」不调用 busy
// handler，那会让「并发建同名卷」的失败方看到 500 而不是 409。
//
// foreign_keys(ON) 在正文库上没有实际作用（那里没有外键）：跨库外键本就不存在，
// 保留同一套 DSN 是为了两库的连接参数只有一处口径。
func openSQLite(ctx context.Context, path string) (*sql.DB, error) {
	dsn := "file:" + path +
		"?_txlock=immediate" +
		"&_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=cache_size(-65536)"
	// 走 observe 的计时包装驱动（DriverName）：DSN 与驱动行为完全不变，只是多一层耗时采集。
	// 打开时 sql.Register 已由 observe 包 init 完成，这里不再依赖下划线导入顺序。
	db, err := sql.Open(observe.DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// 导入是单写者场景，限制写连接数避免 SQLITE_BUSY 抖动。
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck // 打开失败路径，原错误优先
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}
	return db, nil
}

// openStoreSQLite 打开并登记连接池：登记名用于快照里的 Pools（"main" / "content"）。
func openStoreSQLite(ctx context.Context, path, poolName string) (*sql.DB, error) {
	db, err := openSQLite(ctx, path)
	if err != nil {
		return nil, err
	}
	observe.RegisterPool(db, poolName)
	return db, nil
}

// DB 返回主库连接，供同包实现文件与测试使用。
func (s *Store) DB() *sql.DB { return s.db }

// ContentDB 返回正文库连接（chapter_blocks 的唯一宿主）。
//
// 导出给 api / importer 等**同进程**的测试与诊断代码用：线上读写一律走
// Store 上的方法，不要拿它拼自己的正文查询（跨库排序、跨库事务都不成立）。
func (s *Store) ContentDB() *sql.DB { return s.cdb }

// Path 返回主库文件路径。
func (s *Store) Path() string { return s.path }

// ContentPath 返回正文库文件路径。
func (s *Store) ContentPath() string { return s.contentPath }

// Close 关闭主库与正文库（两个都关，任一出错也关另一个）。
func (s *Store) Close() error {
	return errors.Join(s.db.Close(), s.cdb.Close())
}

// Ping 用于健康检查：两个库都要通。
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("store: ping main: %w", err)
	}
	if err := s.cdb.PingContext(ctx); err != nil {
		return fmt.Errorf("store: ping content: %w", err)
	}
	return nil
}

// Checkpoint 把两个库的 WAL 合并回主文件，导入结束后调用可显著缩小体积。
func (s *Store) Checkpoint(ctx context.Context) error {
	errMain := checkpointDB(ctx, s.db)
	errContent := checkpointDB(ctx, s.cdb)
	return errors.Join(errMain, errContent)
}

// checkpointDB 对单个库做 wal_checkpoint(TRUNCATE)。
func checkpointDB(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
