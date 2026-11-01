package store

import (
	"context"
	"database/sql"
	"log"
)

// 0019 的跨库步骤：在正文库建 chapter_fingerprints（章级内容指纹，设计 §4）。
//
// 为什么走 Go 步骤而不是迁移 SQL：表在**另一个数据库文件**里，主库事务建不了它
// （与 0018 拆库同一个理由）。结构仍然只写在 contentSchemaDDL 一处——0018 迁移、
// 0019 迁移、新建库都走 ensureContentSchema，不存在第二份 DDL 口径。
// 主库事务这一路**用不到**（本迁移在主库侧只有 0019 的种子 SQL 那一条设置项）。
//
// 幂等：建表/建索引都是 IF NOT EXISTS，重跑不改任何数据。
// 历史书**不回填**指纹：回填要把几十 GB 正文读一遍解压再哈希，属于离线重建的活；
// 判重对「应用 0019 之后导入的书」生效，历史书按需重导即补齐。
func createFingerprintTableStep(ctx context.Context, _ *sql.Tx, s *Store) error {
	if err := s.ensureContentSchema(ctx); err != nil {
		return err
	}
	n, err := s.fingerprintRowCount(ctx)
	if err != nil {
		return err
	}
	log.Printf("迁移 0019：正文库已建 chapter_fingerprints（当前 %d 行；历史书不回填指纹）", n)
	return nil
}
