package main

import (
	"context"
	"flag"
	"log"
)

// cmdRefreshMetrics 全量重算库级聚合（体积构成缓存）。
//
// 为什么需要它：text_bytes / index_bytes 等是导入时增量累加、启动时后台补算的缓存。
// 迁移（0017 重压、0018 拆库）与 VACUUM/checkpoint 之后，文件大小与块口径都变了，
// 缓存会过期——最糟的情况是 index_bytes 记着迁移中途的 73 GB，让 /api/stats 出现
// 负数或离谱的体积构成。迁移与回收空间之后跑一次这个命令即可。
func cmdRefreshMetrics(args []string) error {
	fs := flag.NewFlagSet("refresh-metrics", flag.ContinueOnError)
	db := fs.String("db", "./data/norvels.db", "SQLite 库路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	st, err := openStore(ctx, *db)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.RefreshMetrics(ctx); err != nil {
		return err
	}
	log.Printf("指标已重算：text_bytes / text_bytes_raw / book_text_bytes / index_bytes / chapter_word_sum")
	return nil
}
