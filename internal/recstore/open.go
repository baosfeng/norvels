package recstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（CGO-free，与主库同一驱动）
)

// EnvRecDB 覆盖推荐库路径（缺省按主库路径推导同目录兄弟文件）。
//
// 与 NORVELS_CONTENT_DB 同一套口径：解析发生在 Open 而非配置包，
// 推荐库必须与主库同目录同寿命，否则备份/迁移会漏一个文件。
const EnvRecDB = "NORVELS_REC_DB"

// Store 是推荐库的单连接池包装。
//
// 只有**一个**库：推荐数据不与主库/正文库做事务编排（设计 §7：两库解耦是前提），
// 因此这里没有 store 那样「双库同寿命」的字段，也不需要跨库补偿。
type Store struct {
	db   *sql.DB
	path string
}

// RecDBPath 由主库路径推导推荐库路径（缺省同目录兄弟文件，NORVELS_REC_DB 整路径覆盖）。
//
// 与 store.ContentDBPath 同一实现口径：norvels.db → norvels-rec.db。保留扩展名
// 是为了「看到 db 就知道是一伙的」，用户在数据目录里一眼能认出三件套。
func RecDBPath(mainPath string) string {
	if v := strings.TrimSpace(os.Getenv(EnvRecDB)); v != "" {
		return v
	}
	mainPath = strings.TrimSpace(mainPath)
	if mainPath == "" {
		return ""
	}
	dir, base := filepath.Split(mainPath)
	if base == "" {
		return "" // 传进来的是目录而不是库文件，交给 Open 报错
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+"-rec"+ext)
}

// Open 打开（不存在则创建）推荐库、应用统一 PRAGMA 并保证 schema 就位。
//
// 独立于 store.Open：推荐库缺失时服务照常启动（推荐端点 503），
// 所以这里的失败不让主库启动路径连坐。
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("recstore: empty database path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("recstore: create dir: %w", err)
	}
	db, err := openSQLite(ctx, path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: path}
	if err := s.ensureSchema(ctx); err != nil {
		db.Close() //nolint:errcheck // 打开失败路径，原错误优先
		return nil, err
	}
	return s, nil
}

// openSQLite 打开 SQLite 文件并应用与 internal/store/open.go **同一套** PRAGMA。
//
// 逐项理由见 store 里的注释（_txlock=immediate 避免快照过期不调 busy handler）；
// 这里刻意复制而不是抽公共包：推荐库与主库的 PRAGMA 口径目前一致，但推荐库整库
// 可重建（坏了直接删文件），将来给它放宽 synchronous 时不该被主库的耐久性绑住。
func openSQLite(ctx context.Context, path string) (*sql.DB, error) {
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
		return nil, fmt.Errorf("recstore: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck // 打开失败路径，原错误优先
		return nil, fmt.Errorf("recstore: ping %s: %w", path, err)
	}
	return db, nil
}

// Path 返回推荐库文件路径。
func (s *Store) Path() string { return s.path }

// Close 关闭推荐库。
func (s *Store) Close() error { return s.db.Close() }

// Ping 用于健康检查与「库还在不在」的探活。
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("recstore: ping: %w", err)
	}
	return nil
}
