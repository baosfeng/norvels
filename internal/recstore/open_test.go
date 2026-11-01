package recstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecDBPathDefaults(t *testing.T) {
	t.Setenv(EnvRecDB, "")
	cases := map[string]string{
		"":                      "",
		"norvels.db":            "norvels-rec.db",
		"/books/norvels.db":     "/books/norvels-rec.db",
		"./data/norvels.db":     "data/norvels-rec.db",
		"/books/library.db":     "/books/library-rec.db",
		"/books/":               "",
		"  /books/norvels.db  ": "/books/norvels-rec.db",
	}
	for in, want := range cases {
		if got := RecDBPath(in); got != want {
			t.Errorf("RecDBPath(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestRecDBPathEnvOverride(t *testing.T) {
	t.Setenv(EnvRecDB, "  /var/lib/rec.db  ")
	if got := RecDBPath("/books/norvels.db"); got != "/var/lib/rec.db" {
		t.Fatalf("环境变量未整路径覆盖: %q", got)
	}
}

func TestOpenCreatesSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "norvels-rec.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeStore(t, s)

	if s.Path() != path {
		t.Errorf("Path() = %q，期望 %q", s.Path(), path)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
	var version int
	if err := s.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读 user_version: %v", err)
	}
	if version != recSchemaVersion {
		t.Errorf("user_version = %d，期望 %d", version, recSchemaVersion)
	}
	for _, table := range []string{"events", "book_stats", "book_daily",
		"user_interest", "cooccur", "recent_items"} {
		if n := countRows(t, s, table); n != 0 {
			t.Errorf("新库 %s 行数 = %d，期望 0", table, n)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "norvels-rec.db")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("首次 Open: %v", err)
	}
	recordEvents(t, first, 1, Event{Kind: KindClick, BookID: 7})
	closeStore(t, first)

	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("重开 Open: %v", err)
	}
	defer closeStore(t, second)
	if n := countRows(t, second, "events"); n != 1 {
		t.Fatalf("重开后 events 行数 = %d，期望保留 1 行", n)
	}
}

func TestOpenEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), "   "); err == nil {
		t.Fatal("空路径应报错")
	}
}

func TestOpenMkdirError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备占位文件: %v", err)
	}
	if _, err := Open(context.Background(), filepath.Join(blocker, "rec.db")); err == nil {
		t.Fatal("父路径是文件时 Open 应报错")
	}
}

func TestOpenPingError(t *testing.T) {
	// 目录当库文件：sql.Open 不报错，Ping 才暴露「打不开数据库」。
	dir := t.TempDir()
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("把目录当库文件打开应报错")
	}
}

func TestOpenMigratesV1ToV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "norvels-rec.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// 造一个「v1 库」：v1 没有负反馈表，版本号也停在 1。
	if _, err := s.db.ExecContext(context.Background(), "DROP TABLE negative_items"); err != nil {
		t.Fatalf("删表: %v", err)
	}
	if _, err := s.db.ExecContext(context.Background(), "PRAGMA user_version = 1"); err != nil {
		t.Fatalf("改版本号: %v", err)
	}
	closeStore(t, s)

	upgraded, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("升级 Open: %v", err)
	}
	defer closeStore(t, upgraded)
	if n := countRows(t, upgraded, "negative_items"); n != 0 {
		t.Fatalf("升级后 negative_items 应为空表，得到 %d 行", n)
	}
	var version int
	if err := upgraded.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if version != recSchemaVersion {
		t.Fatalf("升级后版本 = %d，期望 %d", version, recSchemaVersion)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "norvels-rec.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.db.ExecContext(context.Background(), "PRAGMA user_version = 99"); err != nil {
		t.Fatalf("改版本号: %v", err)
	}
	closeStore(t, s)

	_, err = Open(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "库版本") {
		t.Fatalf("新版本库应被拒绝，得到 err=%v", err)
	}
}

func TestOpenSchemaFailure(t *testing.T) {
	// 预先占用一个索引名，让 createSchema 的建索引语句失败（覆盖「建表失败不落半套表」）。
	path := filepath.Join(t.TempDir(), "norvels-rec.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("预建库: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE idx_events_user_kind (x INTEGER)"); err != nil {
		t.Fatalf("预占索引名: %v", err)
	}
	_ = db.Close()

	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("建索引冲突时 Open 应报错")
	}
}

func TestCreateSchemaOnClosedDB(t *testing.T) {
	s := openTemp(t)
	closeStore(t, s)
	if err := s.createSchema(context.Background()); err == nil {
		t.Fatal("库已关闭时建表应报错")
	}
}
