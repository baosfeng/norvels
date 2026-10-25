package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（CGO-free）
)

// Store 包装一个 SQLite 连接池。
type Store struct {
	db   *sql.DB
	path string
	// metricsRefreshing 去重后台补算（同一时刻只允许一个 goroutine 重算）。
	metricsRefreshing atomic.Bool
	// staleCallback 在后台补算完成后调用（api 层用来记日志），可为 nil。
	staleCallback func(err error)
}

// Open 打开（不存在则创建）数据库并应用 PRAGMA。
// path 为文件路径，例如 ./data/norvels.db。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("store: empty database path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: create dir: %w", err)
	}
	// _txlock=immediate：本库的事务全部是写事务（读走 db.Query，不开事务），
	// 显式起写事务让并发的第二个写者老实等 busy_timeout，而不是拿着旧读快照
	// 在第一条写语句上直接吃 SQLITE_BUSY——WAL 下「快照已过期」不调用 busy
	// handler，那会让「并发建同名卷」的失败方看到 500 而不是 409。
	dsn := "file:" + path +
		"?_txlock=immediate" +
		"&_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=cache_size(-65536)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// 导入是单写者场景，限制写连接数避免 SQLITE_BUSY 抖动。
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// DB 返回底层连接，供同包实现文件使用。
func (s *Store) DB() *sql.DB { return s.db }

// Path 返回数据库文件路径。
func (s *Store) Path() string { return s.path }

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Ping 用于健康检查。
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.db.PingContext(ctx)
}

// Checkpoint 把 WAL 合并回主库，导入结束后调用可显著缩小体积。
func (s *Store) Checkpoint(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
