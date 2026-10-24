# HANDOFF — 交接文档

> 每次阶段性停下前更新本文件。恢复时先读它，再按「下一步」继续。

## 一句话状态

骨架、数据层、解析器、API、部署、文档均已完成并通过测试；**首个 git 提交已做（已脱敏）**；
前端三版原型已出、等拍板后才能进实现阶段。

## 已完成 ✅

| 组件 | 状态 | 证据 |
|---|---|---|
| 项目骨架 + 契约 | 完成 | `docs/architecture.md`、`docs/api.md`、`internal/model/model.go`、`internal/store/types.go` |
| store（SQLite） | 完成，测试绿 | schem 14 表 + 索引 + FTS5 字级分词；`go test ./internal/store/` ok |
| importer | 完成，测试绿 | 8.1 MB/s，60 文件 29s；幂等/SHA256/失败明细/JobID 复用/断点 |
| parser | 完成，测试绿 | 编码自适应、噪声清洗、强/弱两级标题判定、无标记兜底切分、合集标记 |
| api | 完成，测试绿 | 14 端点、中间件、SPA 静态服务；`go test ./internal/api/` ok |
| 部署 | 完成 | `deploy/Dockerfile`（distroless）、`compose.yml`、`scripts/import.sh`、`backup.sh`、`cmd/healthcheck` |
| 文档 | 完成 | `README.md`、`docs/{architecture,api,deploy,research,data-profile,privacy}.md`、`docs/quality-check.sql` |
| 隐私脱敏 + 入库 | 完成 | `docs/privacy.md`、`scripts/privacy-scan.sh`、`make privacy` / `make verify` |
| 前端原型 | 三版骨架已出，待拍板 | `.open-design/03-layouts/{a,b,c}.html` + `03-compare.html/.jpg` |

## 未完成 ⚠️

1. **前端原型待拍板** a/b/c → 然后阶段 4–8（填充/交互/填色/动画/原型）
2. **React 实现未开始**（用户要求先原型后实现，正确）
3. **全量语料导入未跑**（865 文件，预计库体积 ≈ 原文 ×3.3，磁盘预留 ≥20GB）
4. **镜像未实测构建**（本机无 docker/podman/colima）

## 下一步

```bash
cd <仓库根目录>
make verify        # gofmt + go vet + go test + 隐私扫描，应全绿
```

1. 用户拍板原型版本 → 进阶段 4–8 → 出 `.open-design/08-prototype.html`
2. 原型定稿后再写 React（`web/`），用 `odops parity` 对照原型验收
3. 全量导入：`./bin/norvels import --dir "<语料目录>" --db ./data/norvels.db`

## 关键决策记录（别再推翻）

- **搜索 = 字级分词**（unicode61 + content_seg 镜像列），不是 trigram。实测：trigram 2.96x/91s/2字0命中 vs 字级 1.76x/15s/2字命中。
- **库体积真实倍率约 3.3x**（benchmark 1.76x 是重复语料导致的乐观值），`/data` 预留 ≥20GB。
- 作者空 → 统一「佚名」；`author_id=0` 不应出现。
- Go 1.26（x/text v0.43 要求）；Dockerfile `ARG GO_VERSION=1.26`。
- 前端风格 = `warm-editorial`（用户拍板）；先原型后实现。
- **隐私边界**：语料与数据库不入库；文档/原型用虚构书名作者、路径写 `/path/to/txt`；提交前 `make privacy`。

## 环境备忘

- 语料目录：用户本地只读目录（路径不入库，命令行传入即可）
- 本机：go 1.27.1 / node 26.7.0 / pnpm 11.12 / sqlite3 3.54；无容器运行时
- Open Design：`odops` 在 `~/.local/bin/odops`，daemon v0.24.1 在跑
