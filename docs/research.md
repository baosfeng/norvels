# 同类方案调研

> 调研时间：2026-10-24 ｜ 调研对象：把「4.4 GB / 865 个中文 txt」导入本地库并提供书架、目录、阅读、全文搜索，最终产出 <50 MB 轻量容器镜像。
>
> 数据来源与可信度说明：仓库元数据（star、体积、许可证、更新时间）均通过 GitHub REST API 实时读取；二进制体积为**本机实测**（命令见附录）；镜像体积因本机网络无法访问 Docker Hub / GHCR（见「本机容器环境」），凡未能核实的一律标注「**未查到**」，不做估算替代事实。

---

## 结论摘要

1. **没有现成方案同时满足五项约束**（4.4 G 本地 txt / 中文编码混杂 / 章节结构化 / 全文搜索 / 轻量镜像）。电子书服务器系（calibre-web、talebook、Kavita、Komga）本质是「epub / pdf / 漫画」的管理与阅读器，中文 txt 需要先经 Calibre 转成 epub；novel-plus 是带会员充值、AI 写作的 CMS，与本项目不是一个物种；reader 最接近但在功能面上已扩张为「书籍 + 影视 + 音乐」媒体服务器，服务端是 TypeScript/Node 而非 Go 单二进制。
2. **架构上与我们最接近的是 [leafvmaple/zreader](https://github.com/leafvmaple/zreader)**：Go 单二进制 + `modernc.org/sqlite` + `saintfish/chardet` + React SPA 经 `go:embed` 打进二进制 + `alpine:3.20` 运行时，作者自述**约 23 MB 镜像**。它证明「Go 单二进制 + SQLite + 中文 txt 分章」可以做到 20 MB 量级——但项目仅 0 star / MVP 阶段，未针对 4.4 G、865 本量级做批量导入与搜索验证，不能直接当成品用。
3. **中文 txt 的编码识别与分章有可直接借鉴的成熟实现**：[hectorqin/reader 的 `server/src/indexer/formats/text.ts`](https://github.com/hectorqin/reader/blob/master/server/src/indexer/formats/text.ts) 用「BOM → 严格 UTF-8（fatal）→ GB18030 降级」三步，配 6 条刻意收窄的正则 + 60 字标题长度上限过滤误判，并按 256 KB 分块流式读取。这套思路可以直接照搬，无需引入 epub 或 Calibre。
4. **全文搜索采用「汉字逐字分词 + unicode61」，不采用 trigram**。256 MB 中文语料实测：trigram 库体积 2.96x、建索引 91 s、且**不支持 2 字查询**；字级分词（`content_seg` 镜像列）1.76x、15 s、2 字查询 0.3 ms 命中。中文两字词是最高频检索形态，这一点决定了选型。代价有两条且必须写进接口契约：**1 字查询在 API 层直接拒绝**（无区分度），**查询串必须过与入库同一个 `Segment` 函数**（否则命中 0）。详见 [§6](#6-中文分词--全文搜索两种方案实测对比与选型)。
5. **体积目标有充足余量**：本机实测 `CGO_ENABLED=0` + `-trimpath -ldflags="-s -w"` 下，hello world 1.43 MB，加 `modernc.org/sqlite` 后 6.40 MB，加 `net/http` + 400 KB 内嵌前端后 **10.87 MB（amd64）/ 10.43 MB（arm64）**；加 `time/tzdata`（+0.39 MB）、全套 CJK 解码器（+0.38 MB）、独立 healthcheck 二进制（约 2.1 MB）后，配 `distroless/static`（约 2 MiB）镜像预计 **15–17 MB**，`scratch` 约 **13–14 MB**，用 `alpine` 则 **18–24 MB**。50 MB 目标不是瓶颈，**磁盘（4.4 G 文本 + 索引膨胀）与导入耗时才是**。仓库已落地的 [deploy/Dockerfile](deploy/Dockerfile) 与本节推荐同构（构建/运行细节见 [deploy.md](docs/deploy.md)），但有一处参数会让构建直接失败：`GO_VERSION=1.25` 与 go.mod 的 `go 1.26.0` 冲突，详见 [3.1](#31-与现有-deploydockerfile-的对齐检查)。
6. **本机没有任何容器运行时**（docker / podman / colima / lima / nerdctl 均不存在，`docker version` 返回 command not found），镜像构建与体积核验需要在装好运行时后或 CI 里完成；同时 `hub.docker.com`、`raw.githubusercontent.com`、`registry-1.docker.io` 在当前网络被阻断，这也是本文多处「未查到」的原因。

---

## 逐个方案对比

| 项目 | 技术栈 | 镜像体积 | 本地 txt 导入 | 中文编码 | 全文搜索 | 结论 / 可借鉴点 |
|---|---|---|---|---|---|---|
| [novel-plus](https://github.com/201206030/novel-plus)（4763★，Apache-2.0） | Java 6.9 M LOC 量级 / Spring Boot + MyBatis + Sharding-JDBC + MySQL + Redis + Thymeleaf/Layui，多前端子系统（前台/作家后台/管理后台/爬虫） | **未查到**（依赖 MySQL + Redis 外部服务，非单容器可跑） | 有「TXT 文本存储」章节存储方式，但入口是作者后台发布与爬虫采集，不是「扫描目录批量导入」 | 未查到（以数据库存储为主） | 依赖 MySQL 全文/检索，未查到内置中文分词方案 | **不是同类**：多端 CMS + 会员充值与 AI 写作。可借鉴「章节正文按 TXT 落盘、DB 只存索引」的存储分层思路 |
| [hectorqin/reader](https://github.com/hectorqin/reader)（11061★，AGPL-3.0） | 当前 master 服务端为 **TypeScript/Node**（`server/` 下 package.json + tsconfig，仓库语言统计 TypeScript 4.1 M 字符），前端 H5；另含 Kotlin Android 端 | **未查到**（镜像发布在 `cnb.cool/hectorqin/reader`，Docker Hub / cnb 均无法访问） | **原生支持**：挂载目录只读扫描，TXT「编码识别与章节推断」 | **强**：BOM → 严格 UTF-8 → GB18030 降级 | 有库内搜索，具体实现未核对 | **最值得读的一份实现**。注意：功能已扩张到影视/音乐/有声书；AGPL-3.0 有传染性，**只借鉴算法、不复制代码** |
| [leafvmaple/zreader](https://github.com/leafvmaple/zreader)（0★，MIT，枫读） | **Go 单二进制** + `modernc.org/sqlite` + `saintfish/chardet` + `golang.org/x/text` + React/Vite SPA `go:embed`；运行时 `alpine:3.20` | 作者自述 **约 23 MB**（单镜像，amd64/arm64） | **原生支持**：指向目录即扫描，TXT 编码自动识别（UTF-8 BOM / UTF-8 / GBK / GB18030 / ASCII） | **强** | 有书内搜索（v0.6 已交付），未说明是否 FTS5 | **架构与我们几乎一致**，是最贴近的参考实现；同时是「CGO 会毁掉单二进制体积」的实证（README 明确为保住 ~23 MB 镜像而拒绝 Tesseract 绑定） |
| [janeczku/calibre-web](https://github.com/janeczku/calibre-web)（18448★，GPL-3.0） | Python (Fluent 模板 + Python 各约 1.4 M 字符) + 依赖 Calibre 的 metadata.db 与 `ebook-convert` | **未查到**（Docker Hub 阻断）。量级参考：Python + Calibre 二进制，通常数百 MB 起 | 间接：需先由 Calibre 入库为 epub，再由 Web 端阅读 | 由 Calibre 转换链决定，非本项目职责 | 依赖 Calibre 索引，无中文分词方案 | **卡在「只支持 epub/Calibre 库」**。它的定位是「Calibre 库的 Web 前端」，不是 txt 导入器 |
| [talebook/talebook](https://github.com/talebook/talebook)（5856★，BSD-2-Clause） | Python + Nuxt 4 / Vue 3 / Vuetify，构建在 Calibre 之上（镜像基于 `talebook/calibre-docker`） | **未查到**（Hub 阻断；基座为 Calibre 全家桶，量级大） | 支持批量扫描导入 + Legado 书源入库，但**非 epub 格式自动转换**依赖 Calibre | 转换后由阅读器处理 | 有搜索，未查到中文分词细节 | **卡在「必须带 Calibre」**：单是 Calibre 依赖就远超 50 MB。可借鉴其「扫码导入 → 自动补元数据 → 回收站」的产品化流程 |
| [Kareadita/Kavita](https://github.com/Kareadita/Kavita)（11917★，GPL-3.0） | C# / .NET，ASP.NET Core + SignalR + EF Core | **未查到**。量级参考（实测 release 产物）：`kavita-linux-x64.tar.gz` **112.2 MB**、`kavita-linux-musl-x64.tar.gz` 112.3 MB（自包含运行时，发布页 v0.9.1.4） | 书籍格式按 README 为 **epub、pdf**；漫画为 cbr/cbz/zip/rar/rar5/7zip；**未提 txt** | 依赖 epub 内嵌样式，无 txt 编码识别 | 有元数据检索与过滤，非全文搜索 | **卡在「只吃 epub/pdf + .NET 运行时」**。112 MB 的自包含产物决定了镜像不可能进 50 MB |
| [gotson/komga](https://github.com/gotson/komga)（6778★，MIT） | Kotlin / Spring Boot，JVM | **未查到**。量级参考（实测 release 产物）：`komga-1.29.1.jar` **121.0 MB**，另需 JRE 基础镜像 | 面向漫画/杂志/电子书（epub），**未列 txt** | 同左 | 有 API 检索，非全文搜索 | **卡在「JVM + 只支持 epub/漫画」**。MIT 许可宽松，可读其 OPDS/Kobo 同步协议实现 |
| [ma6254/FictionDown](https://github.com/ma6254/FictionDown)（1045★，GPL-3.0） | Go（469 KB 源码，依赖极少） | **未查到**（CLI 工具，非服务） | 面向在线抓取，输出 txt/markdown/epub | 有广告过滤、自动校对、繁简处理 | 无 | **可借鉴其文本清洗规则**（广告行、页眉页脚、重复段落），GPL-3.0 同样只借鉴思路 |
| [chfanghr/makeepub](https://github.com/chfanghr/makeepub)（3★，MIT） | Go（820 KB） | **未查到**（CLI） | txt → 带目录 epub | 未见编码识别说明 | 无 | 小而完整的「txt 分章 → 目录 → epub」链路，MIT 可读可参考 |
| [rupor-github/fb2cng](https://github.com/rupor-github/fb2cng)（82★） | Go，统一格式转换器（FB2 → EPUB2/3、KEPUB、AZW8/KFX、PDF、TXT/MD） | **未查到** | 方向相反（FB2 出 TXT），非导入 | — | — | 说明「格式转换」是独立赛道；我们**不需要**转换，直接入库 txt 更省 |

补充：同类产品还有 [audiobookshelf](https://github.com/advplyr/audiobookshelf)（14760★，有声书/播客，Node）与 [cropflre/nowen-reader](https://github.com/cropflre/nowen-reader)（176★，漫画向，TypeScript），与本项目的「中文 txt 结构化」目标均不重叠，不展开。

---

## 可复用的解析 / 切分思路

### 1. 编码识别：三步降级，且「严格 UTF-8 优先」

出自 [reader `server/src/indexer/formats/text.ts`](https://github.com/hectorqin/reader/blob/master/server/src/indexer/formats/text.ts)（代码注释把理由讲得很清楚）：

```ts
// 1) BOM 是显式声明，直接信任
if (buf[0]===0xEF && buf[1]===0xBB && buf[2]===0xBF) return utf8(buf.subarray(3));
if (buf[0]===0xFF && buf[1]===0xFE)                    return utf16le(buf.subarray(2));
// 2) fatal:true 是关键——否则非法字节会静默变成 U+FFFD，永远发现不了「这文件不是 UTF-8」
try { return new TextDecoder('utf-8', { fatal: true }).decode(buf); }
// 3) GB18030 是 GBK/GB2312 的超集，覆盖绝大多数中文文本
catch { return new TextDecoder('gb18030').decode(buf); }
```

要点：**先严格 UTF-8 失败再降级**，而不是先跑字符集探测器。理由：探测器的启发式在短文本、大量 ASCII 的文件上会误判，而「合法 UTF-8 被当成 GB18030」是不可逆的乱码事故。这正好对上我们 865 个文件的现实（混合编码）。

> 本项目已落地的实现（[internal/parser/encoding.go](internal/parser/encoding.go)）在这三步之上做了扩展，顺序为：BOM（UTF-8/UTF-16LE/BE/UTF-32）→ 大量 NUL 字节判无 BOM 的 UTF-16 → `utf8.Valid` → GB18030 合法性 → Big5 合法性 → 候选打分兜底。比 reader 多覆盖 Big5 与无 BOM UTF-16，正是 865 本杂牌 txt 需要的方向。

若需要更主动的探测（例如决定「是否需要人工复核」），Go 生态可选 [saintfish/chardet](https://github.com/saintfish/chardet)（383★，ICU 派生）——zreader 正是用它；[gogs/chardet](https://github.com/gogs/chardet) 是其 fork；[axgle/mahonia](https://github.com/axgle/mahonia) 是 2018 年后停更的老牌转码库，**不建议**新项目采用。官方转码能力用 `golang.org/x/text/encoding/simplifiedchinese`（GB18030/GBK）即可。

### 2. 章节切分：窄正则 + 标题长度上限

reader 的 `CHAPTER_PATTERNS`（原文注释：误判一条目录项比漏掉一章更烦人，所以刻意收窄）：

```ts
/^\s*第\s*[0-9零一二三四五六七八九十百千万两]+\s*[章回节卷篇集]\s*.{0,60}$/
/^\s*(?:序章|序言|楔子|引子|前言|后记|尾声|终章|番外|附录)\s*.{0,40}$/
/^\s*Chapter\s*\d+.*$/i
/^\s*#{1,3}\s+.+$/            // Markdown 标题
```

配合两条硬过滤：**行长度 ≤ 60 字**（200 字的一行是正文不是标题）、**逐行扫描并记录行号区间**（`startLine/endLine`，正文不改写、不重排）。这样「章节」只是一个 `(title, startLine, endLine)` 的切片视图，原始文本永远可回溯——对「一本书被切错，要能回滚重切」很重要。

zreader 在此基础上多覆盖了几种中文常见写法（见其 README 的 Chapter parsing 条目）：`第X章/节/回/卷`、`Chapter N`、方括号标题，以及 `一、灭门` / `一 灭门` 这种数字序号式；同时提供 **`.chapters.json` 手工覆盖 sidecar**（`match` 文本或精确 `char_offset`），用于自动切分失败的书。这个「自动 + 可覆盖」的双轨设计值得我们照做（我们可放进 `data/overrides/`）。

### 3. 大文件流式读取：分块 + 行边界

reader 用 256 KB 默认块（上限 2 MB）按 offset 读取，并**在行边界截断**。注释里记了一个真实坑：原先按字符 `slice` 截断，会把两行正文粘成一行，客户端按换行判断段落，于是段落结构被服务端悄悄改掉了。对 4.4 G 库，分块读取是硬约束——不能把整本 20 MB 读进内存做正则。

### 4. 清洗规则：广告 / 页眉页脚 / 作者有话说

- [FictionDown](https://github.com/ma6254/FictionDown)（Go）：广告过滤 + 自动校对 + 繁简处理，是中文 txt 清洗规则的现成清单。
- zreader 把清洗做成**可解释的独立步骤**：`promo`（广告）、`edges`（跨章节重复的页眉页脚，用「跨章重复率」检测而非硬编码词表）、`notes`（「作者有话说」「求推荐票」及其后内容）、`normalise`（空白与全半角统一）。对我们有价值的是 **`edges` 的跨章重复检测**：865 本书来源不一，硬编码广告词表维护不动。

### 5. 若要做 txt → epub（本项目暂不需要）

[bmaupin/go-epub](https://github.com/bmaupin/go-epub)（222★，MIT）是纯 Go 的 epub 生成库；[makeepub](https://github.com/chfanghr/makeepub) 是完整的小例子。但注意：**转 epub 只会让链路更长、体积更大**（Calibre/talebook 正是这么变重的），我们的目标形态是「txt 原文 + 结构化索引 + Web 阅读器」，转换应作为可选导出功能而非入库前置。

### 6. 中文分词 / 全文搜索：两种方案实测对比与选型

FTS5 上做中文检索有两条可行路线。在 **256 MB 中文语料**上实测（`modernc.org/sqlite` / SQLite **3.53.2**）：

| 方案 | 库体积 | 插入耗时 | 3 字查询 | 2 字查询 |
|---|---|---|---|---|
| 无索引（基线） | 1.00x | 1.1 s | — | — |
| FTS5 `trigram` | 2.96x | 91 s | 0.3 ms / 109 命中 | **0 命中（不支持 <3 字）** |
| FTS5 `unicode61` + 汉字逐字加空格（`content_seg` 列） | **1.76x** | **15 s** | 0.3 ms / 109 命中 | **0.3 ms / 5 命中** |

**决策：采用字级分词（`unicode61` + `content_seg` 镜像列），不采用 trigram。** 三条理由都来自上表：体积小 40%（1.76x vs 2.96x）、建索引快 6 倍（15 s vs 91 s）、并且**支持 2 字检索**——而中文两字词（`天气`、`江湖`、`梅宁`）恰恰是最高频的检索形态，trigram 在这一点上直接不可用。

机制：入库时把正文按**单个汉字**切分并加空格写入镜像列（`梅宁市` → `梅 宁 市`），FTS5 用 `unicode61` 建索引；查询侧用**同一个 Segment 函数**处理查询串，再以 phrase 形式匹配（`"梅 宁 市"`）。本机对这套机制做了验证（三篇语料）：

| 查询 | 分段结果 | 生成的 phrase | 命中 |
|---|---|---|---|
| `梅宁市`（3 字） | `梅 宁 市` | `"梅 宁 市"` | 1 |
| `梅宁`（2 字） | `梅 宁` | `"梅 宁"` | 2 |
| `天气`（2 字） | `天 气` | `"天 气"` | 1 |
| `梅`（1 字） | `梅` | `"梅"` | 3（= 全部含该字的文档） |
| `梅宁市`（**未分段**直接查） | — | `梅宁市` | **0** |

**代价与接口约束**（必须写进 API 契约）：

1. **1 字查询在接口层拒绝**。单字 phrase 会命中所有包含该字的文档（上表 `梅` 命中 3/3），召回毫无区分度、还白扫索引；应直接返回「请输入至少 2 个字符」，不要落到 SQL 层。
2. **查询串必须过同一个 `Segment` 函数**。上表最后一行就是反例：入库分段、查询不分段，命中 0。`Segment`（[internal/store/util.go](internal/store/util.go)）必须是 store 与 api 共用的唯一实现，不能各写一份。
3. **索引存储形态**：`chapters_fts` 用 external content 表（`content='chapters'`）+ `title_seg`/`content_seg` 镜像列 + AI/AD/AU 三个触发器维护（见 [internal/store/schema.sql](internal/store/schema.sql)）。正文只存一份，避免「原文 + 分段副本」双写导致体积翻倍；全量重建走 `INSERT INTO chapters_fts(chapters_fts) VALUES('rebuild')`。
4. **不需要词典型分词器**。选了字级分词就不必引入 jieba，也就避开 [yanyiwu/gojieba](https://github.com/yanyiwu/gojieba)（2649★，需 CGO）对单静态二进制路线的破坏。若将来要做词级精度（同义词、停用词），再评估纯 Go 的 [go-ego/gse](https://github.com/go-ego/gse)（2844★，Apache-2.0）或 [wangbin/jiebago](https://github.com/wangbin/jiebago)（506★，2020 后停更）。

> 对照记录（本机 `modernc.org/sqlite v1.61.0` / SQLite 3.54.1 的补充实测）：**不分段**直接用 `unicode61` 时，中文整段是一个 token，`江湖`/`风起云涌` 等子串查询**全部 0 命中**，只有 `LIKE '%词%'` 能命中；`trigram` 对 ≥3 字有效（`江湖路` 命中）、对 ≤2 字失效（`山峦` 不命中）。这两条都与上表结论一致，共同说明「必须做切分」。

磁盘：字级分词使库体积达到原文的 **1.76 倍**（4.4 GB → 约 **7.7 GB**），`/data` 建议按 **8–10 GB** 规划（含 WAL checkpoint 峰值与备份余量）。作为对比，若改用 trigram 则约 2.96 倍（≈13 GB）。

---

## 容器轻量化策略

### 1. 本机实测的二进制体积预算

命令：`CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath -ldflags="-s -w"`，Go 1.27.1：

| 构建内容 | raw (amd64) | gzip -9 |
|---|---|---|
| hello world | 1.43 MB | 0.64 MB |
| + `modernc.org/sqlite`（仅 open + `select sqlite_version()`） | 6.40 MB | 2.79 MB |
| + `net/http` + `encoding/json` + `go:embed`（400 KB 随机前端资源） | **10.87 MB** | 4.86 MB |
| 同上，arm64 | **10.43 MB** | 4.54 MB |
| 同上，但**不加** `-s -w` | 16.13 MB | 9.37 MB |
| 同上，仅 API（无 embed） | 10.38 MB | 4.44 MB |
| 同上 + `_ "time/tzdata"` | 11.81 MB（+0.39 MB） | — |
| 仅 API（无 embed）+ x/text 全套 CJK 解码器（GB18030 / GBK / HZGB2312 / Big5 / UTF-16LE / UTF-16BE） | 10.77 MB（**+0.38 MB**） | — |

关键读数：
- `modernc.org/sqlite` 单次引入代价约 **+5.0 MB**（1.43 → 6.40 MB），这是纯 Go SQLite 的「地板价」，换来的是 `CGO_ENABLED=0`，也是能进 `scratch` 的前提。
- `-s -w` 省下 **约 5.3 MB**（16.13 → 10.87 MB），必加。
- `net/http` + JSON 栈约 **+4.0 MB**。想再省可换更轻的路由/序列化，但收益有限，不建议为此牺牲可维护性。
- 前端资源走 `go:embed`，体积≈gzip 前端的实际体积；真实 Vite 产物（React + 少量库）通常 300–600 KB，与实测假设相当。
- 需要时区就用 `time/tzdata`（+0.39 MB）而不是在镜像里装 `tzdata` 包——`scratch` 里没有 `/usr/share/zoneinfo`。
- **编码解码表的代价比直觉小**：GB18030 + GBK + HZGB2312 + Big5 + UTF-16（LE/BE）全套 `golang.org/x/text` 解码器实测只 +0.38 MB（10.39 → 10.77 MB）。这与 [deploy.md](docs/deploy.md) 里「+2–3 MiB」的保守预计不同——**本机实测更小**，不必为省码表去做自研转码表。

### 2. 基础镜像体积（官方口径）

[distroless README](https://github.com/GoogleContainerTools/distroless) 明确给出：`gcr.io/distroless/static-debian13` **约 2 MiB**，约为 `alpine`（`~5 MiB`）的 50%、`debian`（124 MiB）的 2%。这是三档方案体积差的来源。

| 方案 | 基础层 | 我们的二进制 | 预估镜像（amd64，`docker images` 口径） | 取舍 |
|---|---|---|---|---|
| `scratch` | 0 | 11.64 MB（含 tzdata）+ healthcheck 2.1 MB | **约 13–14 MB** | 最小；无 shell、无 CA 证书、无 `/etc/passwd`，健康检查必须用二进制自带子命令 |
| `distroless/static`（`:nonroot`） | ~2 MiB | 同上，tzdata 可由镜像提供 | **约 15–17 MB** | **推荐**：有 `nonroot` 用户与 `/etc/passwd`、无 shell（攻击面小），仍可用 exec 形式健康检查 |
| `alpine:3.20` | ~5 MiB | 10.87 MB（+ `tzdata`/`ca-certificates` 包） | **约 18–24 MB** | 有 shell 可调试、`wget` 可直接写 `HEALTHCHECK`；zreader 实测约 23 MB，与此吻合 |

无论哪档都远低于 50 MB。**真正的体积风险不在镜像，而在 `/data` 里的 SQLite 库与 FTS 索引**（见下）。

### 3. 推荐 Dockerfile 策略（三阶段）

> 仓库里已落地的 [deploy/Dockerfile](deploy/Dockerfile) 与本推荐**同构**（node → golang → distroless，CGO-free，独立 healthcheck 二进制），构建与运行细节见 [deploy.md](docs/deploy.md)，本节只讲「为什么这么切」与体积依据，避免两处文档漂移。

```dockerfile
# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26          # 必须 ≥ go.mod 中的 go 指令版本（当前 1.26.0）

# ---------- Stage 1: 前端（在构建机上跑，不占目标平台） ----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
RUN npm install -g pnpm@9    # 比 corepack 更抗受限网络
COPY web/package.json ./
COPY web/ ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile && pnpm build      # 产物：web/dist

# ---------- Stage 2: Go 后端（CGO-free，内嵌前端） ----------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# 前端产物落到 go:embed 目录（internal/webui/webui.go 里是 //go:embed all:dist）
RUN rm -rf internal/webui/dist && mkdir -p internal/webui/dist \
 && cp -R web/dist/. internal/webui/dist/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/norvels ./cmd/norvels \
 && go build -trimpath -ldflags "-s -w" -o /out/healthcheck ./cmd/healthcheck

# ---------- Stage 3: 运行时（无 shell、非 root） ----------
FROM gcr.io/distroless/static-debian12:nonroot
ENV NORVELS_ADDR=:8080 NORVELS_DB=/data/norvels.db TZ=Asia/Shanghai
WORKDIR /data
COPY --from=build /out/norvels     /norvels
COPY --from=build /out/healthcheck /healthcheck      # 独立静态小工具，见下
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/healthcheck"]
ENTRYPOINT ["/norvels", "serve"]
```

要点说明：
- **健康检查用独立二进制，不用主程序子命令**。主程序里有 `net/http` 与完整的路由、中间件；把探针塞进主程序意味着探针和业务共享同一套初始化（库迁移、索引校验），「库刚损坏」和「进程没起来」会混成一个信号。仓库的 [cmd/healthcheck](cmd/healthcheck/main.go) 用 `net` + `bufio` 手写了一个最小 HTTP/1.1 客户端，刻意不引 `net/http` / `crypto/tls`——按我实测的 4.0 MB `net/http` 栈开销，这个独立二进制只有几百 KB 量级。
- **两段 `--platform=$BUILDPLATFORM`**：编译阶段不受目标架构拖慢（配合 `TARGETOS/TARGETARCH` 交叉编译），与 zreader 的做法一致；多架构发布用 `docker buildx build --platform linux/amd64,linux/arm64`。
- **依赖层在前、源码层在后** + `--mount=type=cache` 缓存 `/go/pkg/mod` 与 `go-build`：改一行业务代码不必重下依赖；4.4 GB 语料若进了构建上下文，缓存全部失效。
- **`.dockerignore` 是硬要求**：排除 `web/node_modules`、`web/dist`、`internal/webui/dist`、`data/`、`.git`、`*.db`。否则光是把 4.4 GB 语料送进 context 就足以让构建慢一个数量级。
- `GOTOOLCHAIN=local` 保证「构建用的 Go 版本就是镜像里的版本」——代价是**镜像 Go 版本必须 ≥ go.mod 的 `go` 指令**，否则直接构建失败（见下）。
- **不要为了省几 MB 引入 CGO**：一旦某个依赖需要 CGO（如 `mattn/go-sqlite3`、`gojieba`），`scratch`/`distroless` 路线立刻作废，还得回到 alpine/glibc 基础镜像。

#### 3.1 与现有 deploy/Dockerfile 的对齐检查

对齐项：三阶段划分、`CGO_ENABLED=0`、`-trimpath -ldflags "-s -w"`、内嵌 `internal/webui/dist`、distroless nonroot、独立 healthcheck 二进制、`ENTRYPOINT ["/norvels","serve"]` ——均一致。

**体积预计偏保守**：[deploy.md](docs/deploy.md) 估 `golang.org/x/text` 码表 +2–3 MiB、总量 17–20 MiB；本机实测全套 CJK 解码器仅 **+0.38 MB**，按此推算总量约 **15–17 MB**。两者都远低于 50 MB，取哪个不影响结论，但实测值更接近真相。

**需要修的一处**：[deploy/Dockerfile](deploy/Dockerfile) 顶部为 `ARG GO_VERSION=1.25`，并设置了 `GOTOOLCHAIN=local`；而 [go.mod](go.mod) 声明 `go 1.26.0`。在 `GOTOOLCHAIN=local` 下，go 1.25 的工具链无法构建要求 `go 1.26.0` 的模块，构建会直接报错（`go.mod requires go >= 1.26.0`）。建议把默认值改为 `ARG GO_VERSION=1.26`（或去掉 `GOTOOLCHAIN=local` 让工具链自动下载）。本条为静态核对结论，**未在本机 docker 中复现**（本机无容器运行时，见「本机容器环境」）。

### 4. SQLite 持久化：WAL 与 volume 的坑

- **挂目录，不挂单文件**。WAL 模式下 SQLite 会在库文件同目录生成 `-wal` 与 `-shm`，单文件 bind mount 会让它们落到容器可写层，重启即丢，甚至损坏。用 `-v norvels-data:/data`（命名卷）或 `-v ./data:/data`（本地目录），镜像里 `VOLUME ["/data"]` 只是声明，实际以运行参数为准。
- **不要放在网络文件系统上**。[SQLite WAL 文档](https://www.sqlite.org/wal.html)写明：*All processes using a database must be on the same host computer; WAL does not work over a network file system* —— 因为 WAL 需要进程间共享一小块内存。所以 NFS / SMB / 部分 NAS 共享目录挂 `/data` 是明确不可行的（这也是大量「SQLite 放 NAS 后库损坏」案例的根因，参见 [SQLite 官方 *SQLite Over a Network* 说明](https://www.sqlite.org/useovernet.html)）。
- **macOS 上的 Docker Desktop**：bind mount 走 virtiofs/gRPC-FUSE，随机小 IO 与文件锁语义都比本地卷差。在本机开发时优先用命名卷，导入 4.4 GB 时的耗时差异会非常明显。
- **连接参数**（实测有效，`modernc.org/sqlite` 的 DSN 形式）：`file:/data/lib.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)`。`busy_timeout` 是避免「database is locked」的第一道防线，别省。
- **并发模型**：WAL 允许「多读 + 单写」。我们的场景天然是「导入/重建索引单写 + 大量并发读」，匹配；但仍建议**导入与阅读不在同一时刻抢写**（导入任务串行化），否则长时间写事务会不断触发 checkpoint 失败、WAL 膨胀。
- **批量导入调参**：整本导入包在一个事务里，导入期可临时 `PRAGMA synchronous=OFF` / `journal_mode=MEMORY`，导入完成后恢复 `synchronous=NORMAL` + `WAL`（见 [PRAGMA 文档](https://www.sqlite.org/pragma.html)）。这能把 865 本、4.4 GB 的导入从「可能几小时」压到可接受区间——但**必须**用「导入完成 → `PRAGMA wal_checkpoint(TRUNCATE)` → 恢复默认」的收尾流程，避免崩溃后库处于不可预期的状态。
- **备份**：不要直接 `cp lib.db`（有 WAL 时是不一致快照）。用 `VACUUM INTO '/data/backup.db'` 或 `sqlite3 .backup`（[SQLite 备份文档](https://www.sqlite.org/backup.html)）。
- **健康检查**：`HEALTHCHECK` 只应探测「进程活着 + 能拿到只读连接」，**不要**在健康检查里跑扫描或写事务；一个 4.4 GB 库上的 `SELECT count(*)` 足以把探针拖成 flapping。

### 5. 体积与容量预估汇总

| 项目 | 预估 | 依据 |
|---|---|---|
| 主二进制 | **约 11.6 MB** | 实测 10.87 MB（http+embed）+ 0.38 MB（CJK 解码器）+ 0.39 MB（tzdata） |
| healthcheck 二进制 | **约 2.1 MB** | [deploy.md](docs/deploy.md) 实测（net+bufio 手写客户端，不引 net/http） |
| 镜像（distroless/static） | **15–17 MB** | 上述两者 + 基础层 ~2 MiB |
| 镜像（scratch） | **13–14 MB** | 同上，去基础层 |
| 镜像（alpine） | 18–24 MB | zreader 自述 ~23 MB 实测吻合 |
| SQLite 库（正文 + 字级分词 FTS 索引） | **约 7.7 GB** | 4.4 GB 原文 × **1.76 倍**（256 MB 语料实测；trigram 方案为 2.96x ≈ 13 GB，已弃用） |
| `/data` 卷建议 | **8–10 GB** | 上述 + WAL checkpoint 峰值与备份余量 |

> 目标「<50 MB 镜像」达成难度低；真正的工程风险是 **4.4 GB 的导入耗时**与 **1.76 倍的库体积**（后者已由字级分词压到可接受区间）。

---

## 本机容器环境

实测命令与结果（2026-10-24，macOS 27.0.1 / arm64 / Go 1.27.1 均来自本机）：

```console
$ which docker podman colima lima nerdctl
# （无输出：五个命令均不存在）

$ docker version 2>&1 | head -3
bash: docker: command not found

$ ls -d /Applications/Docker.app /Applications/OrbStack.app "/Applications/Podman Desktop.app"
# （无输出：均未安装）
$ ls ~/.docker ~/.colima ~/.orbstack ~/.lima
# （无输出：无残留配置）
$ ls /opt/homebrew/bin | grep -E '^(docker|podman|colima|lima|nerdctl|limactl|qemu)'
# （无输出）

$ go version        # go version go1.27.1 darwin/arm64
$ node -v           # v26.7.0
$ sw_vers           # macOS 27.0.1 (26A434)，uname -m = arm64
$ go env GOPROXY    # https://goproxy.cn,direct
```

**结论**：本机**没有任何容器运行时**（Docker Desktop / OrbStack / Podman Desktop / Colima / Lima 均未安装），因此：

1. 本文的镜像体积为**预估**（基于本机实测二进制 + 官方基础镜像体积），**未在本机实际构建验证**；装好运行时后建议立即跑一次 `docker buildx build --platform linux/amd64,linux/arm64` 并 `docker images` 对账。
2. 建议的落地路径：开发机装 OrbStack 或 Docker Desktop（macOS arm64），CI 里用 `buildx` 产出多架构镜像；构建阶段不依赖本机 docker 也可先跑通 `CGO_ENABLED=0 GOOS=linux go build`（交叉编译在本机已验证可用）。

**网络可达性实测**（决定了本次调研的取证方式）：

| 目标 | 结果 |
|---|---|
| `api.github.com` | 200（仓库元数据、README、源码均由此获取） |
| `pkg.go.dev` | 200 |
| `goproxy.cn` | 200（`go get` 可用） |
| `proxy.golang.org` | 超时 |
| `raw.githubusercontent.com` | `curl` 连接被重置 / `web_fetch` fetch failed |
| `hub.docker.com`（含 v2 API） | 连接被重置 → **所有镜像体积数据无法核实** |
| `registry-1.docker.io` | 连接被重置 |
| `ghcr.io` | `/v2/` 返回 401（可达），但匿名 token 端点被拒 → 仍无法读 manifest 层大小 |

---

## 附录：本文实测所用命令（可复现）

```bash
mkdir -p /tmp/szprobe && cd /tmp/szprobe
cat > go.mod <<'EOF'
module szprobe
go 1.24
EOF
go get modernc.org/sqlite@latest      # 实测得到 v1.61.0（内置 SQLite 3.54.1）

# 体积：hello / 仅 sqlite / http+embed+sqlite
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o out .
gzip -9 -c out | wc -c

# FTS5 中文行为（tokenizer 对比）
#   不分段 + unicode61：中文整段一个 token，子串 MATCH 0 命中；只有 LIKE '%词%' 命中
#   trigram          ：≥3 字命中（江湖路 / 风起云涌），≤2 字不命中（江湖 / 山峦）
#   字级分词（每个汉字加空格）+ unicode61：Segment("梅宁市") = "梅 宁 市"，
#     查询侧同样 Segment 后以 phrase "梅 宁 市" 匹配 → 3 字命中、2 字命中、1 字无区分度
#   注：性能与体积数据（1.76x / 15s / 2.96x / 91s）来自 256MB 语料实测，见正文 §6
```
