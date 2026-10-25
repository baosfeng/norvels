# HANDOFF — 项目交接

> 交接时间：2026-10-25 11:00（Asia/Shanghai）｜交接原因：leader 额度不足
> 接手第一件事：读本文 → `git log --oneline | head -20` → `make gate`

## 一句话状态

**代码完成度约 95%，已推 GitHub（`baosfeng/norvels`），CI 全绿，镜像构建还差最后一次验证。尚未发布任何版本。**

## 坐标

| 项 | 值 |
|---|---|
| 本地路径 | 见 `HANDOFF.local.md`（本机文件，不入库） |
| GitHub | `baosfeng/norvels`（公开，main 已推送） |
| Docker Hub | `bsfeng/norvels`（**用户名 bsfeng，与 GitHub 的 baosfeng 不同**） |
| GitHub secret | `DOCKER_HUB_TOKEN`（已配好，名字勿改） |
| 语料目录 | 见 `HANDOFF.local.md`（865 txt / 4.1 GB，只读、不入库） |
| 全量验证库 | `./data/norvels.db`（13.8 GiB，838 本 / 422,715 章，schema v5） |

## 当前 HEAD

`318ceaf` — **36 个提交，每个都能独立编译**（隔离 worktree 逐提交验证过）

---

## 已完成 ✅

### 后端 Go
| 组件 | 覆盖率 | 说明 |
|---|---|---|
| store | 80.2% | 14 表 + FTS5 字级分词，20 个查询方法 |
| parser | 87.8% | **四层可插拔策略管线**（Codec/Filter/Meta/Splitter） |
| importer | 85.1% | 并发 + SHA256 幂等 + 断点续跑 |
| api | 80.1% | 19 端点（含鉴权、驾驶舱 5 个） |
| cli | — | import / serve / stats / reindex / migrate / version |

### 地基（公开发布必需）
- **版本化迁移**：编号迁移 + 事务 + 失败回滚；老库识别为 v1 不重跑建表
- **schema 版本守卫**：库版本 > 二进制版本时拒绝启动，拒绝路径零写入（实测验证）
- **可选鉴权**：`NORVELS_AUTH_TOKEN`，恒定时间比较，`/api/health` 免鉴权
- **配置收敛**：flag > env > 默认，统一 `NORVELS_` 前缀
- **预计算 metrics**：`store_metrics` 表，导入时同事务累加

### 前端 React 19 + Vite + Tailwind
- 五视图 + H5 首屏 + 驾驶舱六图，全部接真实接口；87 测试通过，语句覆盖 88%
- `make build` 产出 12 MB 二进制（前端 go:embed）
- **H5/PC 两套信息架构**：390 下导入/作者/驾驶舱从 DOM 移除（条件渲染，非 CSS 隐藏）
- 微动效三处，尊重 `prefers-reduced-motion`

### CI/CD
- `ci.yml`（4 job）/ `release.yml`（tag → 多架构）/ `edge.yml`（main → edge）
- **CI 全绿**（lint / test / gates / privacy）
- Dockerfile：交叉编译 + distroless debian13 锁 digest

### 质量门禁 `make gate` 8 项全绿
单元测试 / Gherkin 8 场景 / 复杂度（38 项豁免已登记）/ 文件大小 / 依赖方向 / 覆盖率（基线防退化）/ 隐私扫描 / 构建一致性

### 全量语料验证（真实规模）
```
865 文件 / 4.1 GiB → 838 本 / 422,715 章 / 14.1 亿字 / 13.8 GiB
导入 10 分 25 秒，峰值内存 1.28 GiB，失败 0
搜索 3 字 P50 48.9ms；升级 v1→v3 数据完全一致
```

---

## 未完成 ❌

### P0 镜像构建最后一环（唯一阻塞发布）
两次 Edge 失败都已修，**第三次运行尚未验证**：
1. 第一次 `pnpm@9` 读不懂 `allowBuilds`（pnpm 10+ 字段）→ 改 `PNPM_VERSION=11`
2. 第二次 `[ERR_PNPM_IGNORED_BUILDS] esbuild` → **根因是 install 前没复制 `pnpm-workspace.yaml`** → 已修（318ceaf）

**接手第一件事**：看 https://github.com/baosfeng/norvels/actions 最新 Edge run。
- 绿 → 打 tag 发版
- 红 → `ghops actions logs baosfeng/norvels <run-id> --job "<失败job名>"`
- **本机无 docker**，只能用 Actions 验证镜像

### P1 发布第一版
```bash
git tag -a v0.1.0 -m "norvels v0.1.0" && git push origin v0.1.0
curl -s "https://hub.docker.com/v2/repositories/bsfeng/norvels/tags" | python3 -m json.tool
# 确认 amd64 + arm64 都在
```

### P2 解析器两个已知问题（不阻塞）
1. 书名残留 7 本（0.8%），括号不配对；`trimOrphanClosers` 已实现但全量库未重跑验证
2. parser 包 38 个函数复杂度 >10，已登记豁免；重构可参考 `docs/research-parser.md` 级别的调研（legado / zh-chapter-splitter）

### P3 H5 端视觉验收（等用户确认）
首屏顺序（继续阅读 → 搜索 → 最近在读）、书架要不要分页、滑动翻页手感

### P4 可选
`readingBuckets` 聚合端点（前端已写好，字段一上就生效）、`norvels metrics --refresh`、删除书时同步 metrics、门禁 8/9 自动化

---

## 关键技术决策（别推翻）

| 决策 | 理由 |
|---|---|
| 搜索用**字级分词**（unicode61 + content_seg 镜像列） | trigram 对 2 字查询 0 命中且体积 2.96x |
| 库体积约 **3.4x** 原文 | 13.8 GiB / 4.1 GiB，`/data` 预留 ≥20 GB |
| 作者空 → **「佚名」** | 全量语料 52% 无署名，是数据上限 |
| **迁移只前进不回滚** | 回滚须连库一起从备份恢复 |
| **单用户无账号体系** | `NORVELS_AUTH_TOKEN` 口令级门禁；公网必须设 |
| **预计算 metrics** | `SUM(length(content))` 54s vs `SUM(word_count)` 0.05s |
| **不引图表库** | 手写 SVG/CSS，保单文件与镜像体积 |
| 原型先于实现 | 用户要求，已完成 |

---

## 踩过的坑（别重复）

1. **`CREATE TABLE IF NOT EXISTS` 改不了已有表结构** — store_metrics 踩坑，改 `DROP + CREATE`（表里都是可重算的派生数据）
2. **本地跑通 ≠ 容器跑通** — pnpm 版本、workspace 复制，两次都是本地无感、CI 才炸。已沉淀为门禁第 10 项
3. **「失败 0」可能是把劣质切分静默吞了** — parser 发现两个丢正文 bug（某书丢 57 万字、某书恢复 134 万字），被「整体回退兜底」掩盖
4. **多 agent 并发写同一工作区会互相干扰** — 逐提交验证要用 `git worktree add --detach`
5. **门禁豁免按「函数名+文件」匹配**，不按行号

---

## 环境备忘

- Go 1.27.1 / node 26.7 / pnpm 11.12 / sqlite3 3.54 / **无 docker**
- `ghops`（`~/.local/bin/ghops`）操作 GitHub，不是 gh CLI
- `odops`（`~/.local/bin/odops`）Open Design，daemon 需在跑
- 隐私扫描 `scripts/privacy-scan.sh --all`，命中 exit 1；黑名单 `.privacy-denylist.txt`（不入库）
- **推送前必须跑 `make gate`**

---

## 隐私边界（重要）

- 语料、数据库、原型截图、Open Design 种子**都不入库**
- 文档与原型示例一律用**虚构书名/作者**，路径写 `/path/to/txt`
- 真实书名/作者/站点名登记在 `.privacy-denylist.txt`（本机，不入库）
- 完整规则见 `docs/privacy.md`
