# norvels — 本地中文小说库

> Go 1.26（`CGO_ENABLED=0`）单二进制 + SQLite（正文按 1 MB 块 zstd 压缩）+ React 19 内嵌前端；当前 v0.6.0，生产跑在飞牛 NAS（部署见 [deploy/nas](deploy/nas/README.md)）。

## 先读文档，再动代码

> 改解析→[parser-architecture](docs/parser-architecture.md)｜改数据层/迁移→[architecture](docs/architecture.md)+[ADR](docs/adr/)｜改接口→[api](docs/api.md)｜改前端/图表→[charts.ts](web/src/lib/charts.ts)｜部署升级→[deploy](docs/deploy.md)｜发版→[release](docs/release.md)｜门禁口径→[quality-gates](docs/quality-gates.md)｜全量导航→[README](README.md)

## 隐私红线（不进仓库）

语料、数据库、真实书名/作者、截图都留在本地——代码、文档、测试、示例、提交信息一视同仁。示例用虚构书名与 `/path/to/txt`；真实词表在 `.privacy-denylist.txt`（本机、已 gitignore）。提交前跑 `scripts/privacy-scan.sh --all`。

## 构建与验证

```bash
make build        # 前端 + 后端 → bin/norvels
make gate         # 提交前必跑
```

> 门禁 9 项：单元（gofmt + vet + go test）/ Gherkin / 圈复杂度 ≤10 / 函数 ≤70 行 / 文件 ≤400 行 / 依赖方向 / 覆盖率基线 / 隐私 / 前端 vitest。豁免按「函数名 + 文件」登记在 quality-gates.md。每个提交要能独立编译（`git worktree add --detach` 验证）。

## 关键约束

- 迁移只前进：回滚 = 连数据库一起从备份恢复。
- 大库迁移 20+ 分钟且期间不监听端口：用一次性容器预跑 `docker run --entrypoint /norvels … migrate`，避免 healthcheck 在迁移中途重启它。
- 迁移后必须 VACUUM，否则文件不缩小（旧数据只变成空洞页）。
- 容器内路径是 `/books`（宿主路径在容器里不存在）；数据目录属主 65532；镜像 ENTRYPOINT 是 `norvels serve`，跑子命令要 `--entrypoint /norvels`。
- 镜像 tag 只发 `X.Y.Z` + `latest`，不生成 `x.y` / `x` / `sha-*` / `edge`。
- 依赖方向由门禁强制，新增内部包先在 `tests/tools/gatedeps/main.go` 登记。
- **改动收尾即清理**：被取代的文档、失效的引用与链接、临时二进制、构建缓存、遗留脚本随本次改动一起删；文档里的数字按新实测更新，不留旧值。

## 文档规则

单文档 ≤ 120 行，一屏读完；只写读者读不到的判断；同一信息只出现一次；规则用「推荐 / 默认 / 优先」正向表述。
