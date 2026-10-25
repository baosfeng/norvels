-- 0006 移除字级分词镜像列与正文全文索引。
--
-- 背景：用户明确「只涉及读取章内容，不需要按内容查找」。字级分词是纯为「正文检索」
-- 服务的两坨数据，正文检索取消后没有读者：
--   chapters.content_seg / title_seg  逐字加空格的正文副本（全量库实测 6.28 GB）
--   chapters_fts + 三个同步触发器      为它建的 FTS5 倒排索引（实测 3.30 GB）
--
-- DDL 分两层（分工原因见下）：
--   1. 本文件里的语句全部可条件化（IF EXISTS），在「chapters 表还不存在」的畸形
--      老库升级路径上也能跑通；
--   2. 「去掉 title_seg / content_seg 两列」无法条件化（SQLite 没有条件 ALTER），
--      由 Go 在**同一个迁移事务**内幂等执行：见 migrate.go 的 runMigrationSteps
--      → dropSegColumns。放进同一事务是为了守住「一条迁移一个事务、失败整体回滚」，
--      否则 SQL 提交成功、后续步骤失败会留下版本号已前进的半迁移库。
--
-- 顺序要求：删触发器必须早于删表。触发器体引用了 chapters_fts，先删表会留下
-- 「指向不存在表」的残骸——SQLite 允许它存在，但之后任何 chapters 写入都会
-- 报 no such table: chapters_fts。这里显式先删三个触发器，不依赖 DROP TABLE 的副作用。
DROP TRIGGER IF EXISTS chapters_ai;
DROP TRIGGER IF EXISTS chapters_ad;
DROP TRIGGER IF EXISTS chapters_au;

-- 影子表（chapters_fts_data / _idx / _docsize / _config）随虚拟表一并删除。
DROP TABLE IF EXISTS chapters_fts;
