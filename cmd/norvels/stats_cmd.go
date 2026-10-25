package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func cmdStats(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	fs.Parse(args)
	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	// 先做一次全量重算：stats 只读预计算缓存，缓存缺失时（例如刚迁移完、
	// 或口径版本刚变更）会显示全 0。CLI 是运维入口、不在请求路径上，
	// 重算代价可接受，而显示错误的体积构成比慢一点严重得多。
	if err := st.RefreshMetrics(ctx); err != nil {
		return err
	}
	s, err := st.Stats(ctx)
	if err != nil {
		return err
	}
	w := os.Stdout
	fmt.Fprintf(w, "书籍: %d\n章节: %d\n作者: %d\n字数: %d\n库大小: %s\n",
		s.Books, s.Chapters, s.Authors, s.Words, humanBytes(s.DBBytes))
	fmt.Fprintf(w, "体积构成（估算）: 正文 %s / 索引 %s / 其他 %s\n",
		humanBytes(s.TextBytes), humanBytes(s.IndexBytes), humanBytes(s.OtherBytes))
	if s.LastImportAt != nil {
		fmt.Fprintf(w, "最近导入: %s\n", s.LastImportAt.Format(time.RFC3339))
	}
	return nil
}

// cmdMigrate 单独执行迁移；--dry-run 只打印将要应用的迁移，不写库。
