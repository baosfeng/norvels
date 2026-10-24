package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（CGO-free）
)

//go:embed schema.sql
var schemaSQL string

// Store 包装一个 SQLite 连接池。
type Store struct {
	db   *sql.DB
	path string
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
	dsn := "file:" + path +
		"?_pragma=busy_timeout(10000)" +
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

// Migrate 应用 schema（幂等：全部使用 IF NOT EXISTS）。
func (s *Store) Migrate(ctx context.Context) error {
	if strings.TrimSpace(schemaSQL) == "" {
		return fmt.Errorf("store: schema.sql is empty")
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

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
