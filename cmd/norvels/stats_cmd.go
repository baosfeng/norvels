package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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

func cmdReindex(args []string) error {
	cfg := mustConfig()
	fs := flag.NewFlagSet("reindex", flag.ExitOnError)
	dbPath := fs.String("db", cfg.DB, "数据库路径")
	fs.Parse(args)
	ctx := context.Background()
	st, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	start := time.Now()
	if err := st.Reindex(ctx); err != nil {
		return err
	}
	log.Printf("索引重建完成，用时 %s", time.Since(start).Round(time.Second))
	return nil
}

// cmdMigrate 单独执行迁移；--dry-run 只打印将要应用的迁移，不写库。
