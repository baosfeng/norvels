package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// query 是一条待执行的块读取语句（抽样时按 book_id 分块，避免变量数上限）。
type query struct {
	sql  string
	args []any
}

// plan 描述这次要扫哪些块。
type plan struct {
	mode    string
	books   int
	blocks  int
	queries []query
}

// openReadOnly 以只读方式打开库。
//
// 必须用 mode=ro 而不是 immutable=1：生产库开着 WAL，immutable 会让 SQLite 无视
// -wal 里尚未 checkpoint 的页，读到的可能是旧状态；mode=ro 只是不许我们写，
// 写相关的 pragma（journal_mode / synchronous）一律不加，避免在只读库上触发写入。
func openReadOnly(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?mode=ro&_pragma=busy_timeout(10000)&_pragma=temp_store(MEMORY)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开库失败: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("只读打开失败（路径 / 权限 / 无 -shm 权限？）: %w", err)
	}
	return db, nil
}

// chunkBooks 每批放多少个 book_id 进 IN 列表（远低于 SQLite 变量上限）。
const chunkBooks = 400

// contentDBPath 由主库路径推导正文库路径（缺省同目录兄弟文件，NORVELS_CONTENT_DB 覆盖）。
//
// 与 internal/store.ContentDBPath 是同一规则的两份实现：门禁要求 tests/ 下的工具包
// 只依赖标准库（见 tests/tools/gatedeps），所以不能 import store 复用。
// 两份的用例表必须逐条一致（本包 TestContentDBPathDerivation ↔ store 同名测试）。
func contentDBPath(mainPath string) string {
	if v := strings.TrimSpace(os.Getenv("NORVELS_CONTENT_DB")); v != "" {
		return v
	}
	dir, base := filepath.Split(mainPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem, ext = base, ""
	}
	return filepath.Join(dir, stem+"-content"+ext)
}

// resolveContentPath 定位正文库：显式给了就用，否则按主库路径推导。
//
// 推导出来的文件不存在时给出可执行的提示：0018 之前的库块表还在主库，
// 直接抛 sqlite 的 "unable to open database file" 会让运维以为工具坏了。
func resolveContentPath(dbPath, contentDB string) (string, error) {
	if contentDB != "" {
		return contentDB, nil
	}
	derived := contentDBPath(dbPath)
	if _, err := os.Stat(derived); err != nil {
		return "", fmt.Errorf("正文库不存在：%s（0018 之前的库块表在主库，请先迁移或显式指定 -content-db）", derived)
	}
	return derived, nil
}

// planBlocks 决定这次扫全库还是抽样，并把查询拆好。
//
// 块数与块读取走**正文库**（0018 起 chapter_blocks 只在那里）；书数与抽样 id 走主库。
func planBlocks(main, content *sql.DB, cfg config) (plan, error) {
	if cfg.all {
		n, err := countRows(content, "SELECT COUNT(*) FROM chapter_blocks", nil)
		if err != nil {
			return plan{}, err
		}
		books, err := countRows(main, "SELECT COUNT(*) FROM books", nil)
		if err != nil {
			return plan{}, err
		}
		return plan{
			mode:   "all",
			books:  books,
			blocks: n,
			queries: []query{{
				sql: "SELECT id, packed FROM chapter_blocks ORDER BY id",
			}},
		}, nil
	}

	ids, total, err := sampleBookIDs(main, cfg.sample)
	if err != nil {
		return plan{}, err
	}
	qs := bookQueries(ids)
	blocks := 0
	for _, q := range qs {
		n, err := countRows(content, "SELECT COUNT(*) FROM chapter_blocks WHERE book_id IN ("+placeholders(len(q.args))+")", q.args)
		if err != nil {
			return plan{}, err
		}
		blocks += n
	}
	return plan{
		mode:    fmt.Sprintf("sample %d/%d", len(ids), total),
		books:   len(ids),
		blocks:  blocks,
		queries: qs,
	}, nil
}

// sampleBookIDs 按 id 等间隔取样本：可复现，且不依赖随机源的版本差异。
func sampleBookIDs(db *sql.DB, n int) ([]int64, int, error) {
	rows, err := db.Query("SELECT id FROM books ORDER BY id")
	if err != nil {
		return nil, 0, fmt.Errorf("读 books 失败: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var all []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("扫 books.id 失败: %w", err)
		}
		all = append(all, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if n <= 0 || n >= len(all) {
		return all, len(all), nil
	}
	picked := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		picked = append(picked, all[i*len(all)/n])
	}
	return picked, len(all), nil
}

// bookQueries 把书 id 切成 IN 查询（按 book_id, seq 排序，保证多次运行同序）。
func bookQueries(ids []int64) []query {
	var qs []query
	for start := 0; start < len(ids); start += chunkBooks {
		end := start + chunkBooks
		if end > len(ids) {
			end = len(ids)
		}
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		qs = append(qs, query{
			sql: "SELECT id, packed FROM chapter_blocks WHERE book_id IN (" +
				placeholders(len(args)) + ") ORDER BY book_id, seq",
			args: args,
		})
	}
	return qs
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	buf := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '?')
	}
	return string(buf)
}

func countRows(db *sql.DB, sqlText string, args []any) (int, error) {
	var n int
	if err := db.QueryRow(sqlText, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("计数失败: %w", err)
	}
	return n, nil
}
