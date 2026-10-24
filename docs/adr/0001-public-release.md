# ADR-001：公开发布与可升级架构

> 决策日期 2026-10-24。触发：确定要推 GitHub → GitHub Actions → Docker Hub 公开发布，
> 且后续会持续加功能。以下决定在后续实现中默认成立，改变需新开 ADR。

## 1. 交付形态

| 项 | 决定 |
|---|---|
| 分发渠道 | Docker Hub 单一公开镜像（`<owner>/norvels`），多架构 amd64 + arm64 |
| 部署模型 | 单容器 + 一个 `/data` 卷；前端 embed 进二进制，不分开发布 |
| 版本号 | 语义化版本；git tag → 镜像 tag；`main` 分支 → `edge` |
| 数据库 | 单 SQLite 文件，随卷持久化；升级时原地迁移 |

## 2. 升级能力（本次要补的地基）

**版本化迁移** —— 当前 `Migrate()` 每次执行整份 `schema.sql`（幂等 IF NOT EXISTS），
只能加表不能改表，后续加字段/改索引会被卡住。改为：

- `internal/store/migrations/` 放编号迁移：`0001_init.sql`、`0002_reading_progress.sql`…
- `schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT)` 记录已应用版本
- 启动时按序应用「大于当前版本」的迁移，单个迁移在一个事务内
- `//go:embed migrations/*.sql` 打包；迁移只前进不回滚（回滚靠备份，见 docs/deploy.md）
- 迁移失败 → 启动失败并打印版本号，**不带着半迁移的库继续跑**
- 提供 `norvels migrate --dry-run` 打印将要应用的迁移

判据：拿 v1 老库（`PRAGMA user_version=1`）启动新版本，应自动补到最新版本且数据不变。

## 3. 公开暴露的安全边界（本次要补）

镜像公开 + 可能部署在公网 VPS ⇒ **默认必须能开鉴权**。

- 环境变量 `NORVELS_AUTH_TOKEN`：设置后所有 `/api/*` 与页面都要求 Bearer Token 或 Cookie
- 未设置时 = 开放模式（本地/内网使用），启动日志 **显式警告**「未启用鉴权，请勿暴露到公网」
- 不做账号体系（单用户），只做「一个共享口令」级别的门禁
- 令牌比较用恒定时间比较；Cookie 用 HttpOnly + SameSite=Lax
- `/api/health` 在鉴权模式下仍免鉴权（供容器健康检查）

## 4. 配置

优先级：命令行 flag > 环境变量 > 默认值。统一前缀 `NORVELS_`。

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `NORVELS_ADDR` | `:8080` | 监听地址 |
| `NORVELS_DB` | `/data/norvels.db` | 数据库路径 |
| `NORVELS_AUTH_TOKEN` | 空 | 空 = 开放模式 |
| `NORVELS_LOG_LEVEL` | `info` | debug/info/warn/error |
| `NORVELS_BOOKS_DIR` | `/books` | 导入源目录（只读挂载点） |

## 5. CI/CD

- `ci.yml`：PR 与 main 上跑 gofmt / vet / go test / gate（复杂度、大小、依赖、覆盖率）/ 隐私扫描
- `release.yml`：推 `v*` tag → buildx 多架构 → Docker Hub；打 `latest` + 精确版本 + `vX.Y` + `vX`
- `edge`：main 分支每次通过 CI 后推 `edge` tag（可关闭）
- 所需仓库 secret：`DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`
- 不在 CI 里跑需要真实语料的测试（`TestSampleRealCorpus` 靠环境变量，默认 skip）

## 6. 前端图表（驾驶舱）

用户要求「图表越多越好，信息密度高」。约束：

- 不引图表库（保持单文件自包含与镜像体积）——用 SVG / CSS 手写
- 每张图必须有真实数据来源；没有接口的图 **先不画**，等接口就绪
- 深色/浅色两套主题下都要可读

驾驶舱图表清单（配套接口见 docs/api.md）：
1. 近 N 天导入量（条形）
2. 章节数分布（直方图：按书统计章节数分桶）
3. 字数 Top 10 作者（横条）
4. 阅读进度分布（环形：未开始 / 在读 / 读完）
5. 库体积构成（堆叠条：正文 / 索引 / 其他）
6. 章节长度分布（折线：按章统计字数分桶）