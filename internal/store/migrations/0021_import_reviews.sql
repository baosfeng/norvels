-- 0021 AI 审核表（设计 §6）与「版本替换」列（设计 §7）。
--
-- 两处结构变更都不是能写进本文件的 DDL：
--   · CREATE TABLE import_reviews + 索引；
--   · ALTER TABLE books ADD COLUMN superseded_by：SQLite 没有 ADD COLUMN IF NOT EXISTS，
--     而「删掉版本号重跑」是排查迁移问题的常规手段，撞 duplicate column 会让整次迁移失败。
-- 它们由 Go 步骤 migrationStepsByVersion[21] -> createImportReviewsStep 在**同一事务**内
-- 幂等执行（与 0008 / 0010 / 0011 / 0020 同一条路径）。本文件的 SQL 只做能落在主库数据上的部分。
--
-- superseded_by 的语义：0 = 当前版本（书架、详情、阅读可见）；非 0 = 被这个 book id
-- 取代的旧版本（不可见，块与章节都保留，供回滚）。导入判出「升级候选」时新书先以
-- superseded_by = 旧书 id 入库（书架不会多出一本重复的书），人工确认替换后两者互换，
-- 回滚再换回来。任何自动替换都不做（设计 §7）。
--
-- 预置 AI 开关：默认关闭（设计 §6「可关闭」）。关闭时导入仍会记录审核候选，但后台
-- worker 直接把记录置 needs_manual，不外发任何正文。
-- 幂等：ON CONFLICT DO NOTHING —— 用户手工打开后重跑不会把值打回 false。
INSERT INTO settings(key, value, updated_at)
VALUES ('ai.enabled', 'false', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(key) DO NOTHING;
