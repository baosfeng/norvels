# 常见问题（FAQ）

> 读者真会问、且答案需要「先给结论再给排查顺序」的问题都在这里；README 只保留一句指路。
> 分工：运维 / 部署 / 备份细节以 [deploy](deploy.md) 为准，阅读器行为与边界以 [reading](reading.md) 为准，本文只做入口与最短答案。

## 导入与解析

### 中文乱码怎么办？

服务端按 UTF-8 输出 JSON，终端 locale 不影响数据；入库时 parser 对每个文件做多候选解码（utf-8 / utf-8-bom / gb18030 / utf-16le / utf-16be / big5），按「CJK 命中率 − U+FFFD 惩罚」选最高分，不依赖文件头。

1. `./bin/norvels import --dry-run --dir "/path/to/txt"` 先试跑，看该文件的 `Warnings`；
2. `GET /api/books/{id}` 的 `encoding` 字段是探测结果，可据此判断该文件按哪种编码入库；
3. 仍有个别段落乱码，多为「GBK 主文 + 少量 UTF-8 广告行」的混合编码，v1 允许残留少量 U+FFFD，要求行级可读。

机制与候选集见 [parser-architecture](parser-architecture.md)；全量语料编码分布见 [data-profile §2](data-profile.md)。

### 章节没有切开？

切章是启发式打分：一组 Splitter（`di-unit` / `special` / `numeric` / `en-chapter`）各自算适配度，取最高分执行，全都不认领时回退 `unmarked` 兜底，保证至少能入库。

1. `--dry-run` 看该文件的 `Warnings`，确认命中的是哪个策略或落到兜底；
2. 标题行可能被广告清洗规则误删（含网址 / 群号 / 水印的短行会被清掉）；
3. 「多书合集」由 `SplitSegments` 按章号重置点拆成多本（每段至少 3 章），拆不开时按单本导入并在 `Warnings` 里说明。

策略清单与降级语义见 [parser-architecture §4](parser-architecture.md)。

## 部署与运维

### 镜像为什么这么小？

三阶段构建 + `CGO_ENABLED=0`（纯 Go SQLite）+ `-trimpath -ldflags "-s -w"` + distroless/static（无 shell、apt、curl）。
体积：二进制 **16.29 MiB**、healthcheck **2.14 MiB**、基镜像解压后 **3.27 MiB**，合计约 **21.7 MiB**（目标 < 50 MiB）。
前端产物走 `go:embed` 进二进制，不额外占镜像层。逐项实测与构建命令见 [deploy §3](deploy.md#3-镜像体积拆解)。

### 备份要不要连 WAL 与正文库一起备？

**请用 `scripts/backup.sh`，别直接 `cp` 主库**：WAL 模式下未 checkpoint 的数据在 `-wal` 里，直接拷主库会丢最新数据，也会漏掉整个正文库。

```bash
scripts/backup.sh --keep 7   # .backup 或 VACUUM INTO，自动成对备份两个库
```

- **主库 `norvels.db` + 正文库 `norvels-content.db` 必须成对备份、成对恢复**（只恢复主库 = 全部正文丢失）；保留策略按「一对」计算。
- 推荐库 `norvels-rec.db` 存推荐与埋点数据，**可整库重建，不进备份**；丢了只是推荐页降级。
- 恢复：停服务 → 成对替换两个库文件 → 删掉遗留的 `-wal` / `-shm` → 启动。
- 导入期间不并发备份（`VACUUM INTO` 要写锁，会放大 IO）；升级前那次备份另存到数据卷之外。

部署侧完整口径见 [deploy §5](deploy.md#5-sqlite-on-volume-注意事项) 与 [§7.1](deploy.md#71-铁律迁移只前进回滚必须连数据库一起恢复)。

## 阅读体验

### 手机上为什么不能选中正文复制？

分页模式下正文容器是 `user-select: none`（[ix-reader-paging.css:27-28](../web/src/styles/ix-reader-paging.css)），横向拖动优先给翻页引擎，选词会让「拉一下翻页」变成「拖蓝一段」。
要复制或选中去问 AI，把阅读模式切到「上下滚动」即可，选中能力与 PC 一致；边界见 [reading](reading.md#读者能感知的契约)。

### 换了设备为什么背景色没跟着走？

主题存在浏览器本地 `localStorage` 的 `norvels.theme` 键（[theme.ts:12](../web/src/lib/theme.ts)），只有**排版设置**（字号 / 行距 / 段距 / 页宽等）按账号同步到服务端；换设备或换浏览器后背景色默认回「日间」，重选一次即可。
设计稿里写过「主题也同步」，以代码为准，见 [reading](reading.md)。

### 为什么从目录点回以前读过的章，位置在章首？

进度是「章 + 段」的段级记忆，但后端每本书只留**一条**最后位置（`reading_progress` 以 `(user_id, book_id)` 为联合主键覆盖写，[progress.go:104](../internal/store/progress.go)）。
所以回看别的章一律落章首，只有「上次那一章的那一段」能精确恢复——这是当前口径，不是偶发 bug；见 [reading](reading.md#读者能感知的契约)。

### 能按正文内容搜书吗？

不能，**检索只匹配书名**（`LIKE` 子串）：作者名 LIKE 没有索引可走，在大库上是全表扫描的主要成本，找某位作者的书从结果里的作者名点进作者页（[book_filter.go](../internal/store/book_filter.go)）。章内搜索是纯前端、只匹配当前这一章。
正文全文索引已主动移除：早期「字级分词镜像列 + FTS5」占掉全库 **65%** 体积，读者实际按书名找书。决策与代价见 [ADR-0002](adr/0002-drop-fulltext-index.md)。

### 阅读时长为什么比我实际读的少？

计时器只在页面可见时累计，且**单次停留 ≥ 60 秒才上报**；切章 / 页面隐藏 / 卸载时才落一次库，不足 60 秒的那段直接丢弃。
连续读满 5 分钟会兜底落一次，所以进程被杀也不会全丢；累计口径见 [reading](reading.md)。

### 为什么 PC 上没有左右翻页？

分页与五种翻页效果只在 H5 挂载：判定收在一处 `pageModeFor(isH5, settings)`，`isH5` 是 `max-width: 768px`，PC 恒为连续滚动。
手机上选的分页方式与效果在 PC 不生效但值保留，回到手机仍是那一套；手势与几何见 [h5-paged-reading](specs/2026-11-04-h5-paged-reading.md)。
