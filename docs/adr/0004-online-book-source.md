# ADR-004：在线书源抓取与规则引擎选型

> 决策日期 2026-10-31。触发：用户拍板本期做「本地规则包 + 在线书源抓取」，要求先拿数据判断是否支持 JS。
> 原始统计与实测见 `/tmp/online-source-research.md`（不入仓库）；书源语法与字段对照见 [source-pluggable](../source-pluggable.md)。

## 1. 场景与约束

norvels 的产品前提是「单二进制 + distroless + 不联网」，但用户在 NAS 上要按来源抓书，
在线抓取因此成为**显式的、默认关闭的能力**，而不是产品前提的改变：

| 约束 | 对设计的影响 |
|---|---|
| 单二进制、`CGO_ENABLED=0`、distroless | 不能 dlopen、不能依赖外部运行时；JS 引擎只能是纯 Go 库 |
| 家庭共享口令（无密码、内网私有） | 书源是「用户自己导入的配置」，仍需按不可信输入对待（规则来自网络） |
| NAS 常驻、865 文件级真实库 | 抓取必须限速、限并发，不能把导入任务挤死 |
| 已入库的书不可被在线抓取污染 | 在线结果只允许「新建 / 追更（只追加）」，不覆盖已有章节 |

**与「不联网」的关系**：默认 `off`；开启后只允许用户显式配置的书源域名出网，
不存在后台常驻抓取，也不做书源市场自动更新。

## 2. 书源模型：与本地规则包共用 settings 里的 `source_profiles`

存 `settings` 表的 `source_profiles` 键（JSON 数组、全量替换，与 `chapter_rules`/`tag_rules` 同形，不新开表）：校验、启停、优先级、导入导出、试跑都复用；在线差异收进 `online` 子对象，本地规则包不受影响。
需要按 `id` 查询或加 revision 历史时再迁移成表——一次迁移即可，书源 JSON 形状不变。

| Legado 字段 | norvels 映射 | 说明 |
|---|---|---|
| `bookSourceUrl` | `online.baseUrl`（同 `id` 唯一键） | 重复导入覆盖同源，与 Legado 一致 |
| `bookSourceName/Group` | `name` / `group` | 分组只影响列表展示与筛选 |
| `enabled/weight` | `enabled` / `priority` | 优先级参与同书多源选源 |
| `searchUrl` / `exploreUrl` | `online.searchUrl` / `online.exploreUrl` | 支持 `{{key}}` `{{page}}` 两个变量 |
| `ruleSearch` / `ruleExplore` | `online.rules.search` / `explore` | 列表字段：name/author/bookUrl/coverUrl/intro/kind |
| `ruleBookInfo` | `online.rules.bookInfo` | 详情字段 + `tocUrl` |
| `ruleToc` | `online.rules.toc` | chapterList/chapterName/chapterUrl |
| `ruleContent` | `online.rules.content` | 正文 + `nextContentUrl`（分页正文） |
| `header` / `loginUrl` / `cookie` | **本期不支持** | 需要登录态的站点不支持，写入时 `unsupported_fields` 提示 |

## 3. 抓取执行器安全边界

| 面 | 做法（可落地） |
|---|---|
| 协议 | 仅 `http`/`https`；`file:` `gopher:` `data:` `ftp:` `javascript:` 直接拒绝 |
| 端口 | 出网端口默认只允许 80/443，**规则包内不能放宽**；需要非标准端口时改服务端配置（`AllowedPorts`）并重启 |
| SSRF 第一道 | 非 IP 的 host 先 `LookupIPAddr`，**所有**解析结果校验；任一命中私网即拒绝 |
| SSRF 黑名单 | 回环 / 私网 / 链路本地 / 未指定 / 组播；`169.254.0.0/16`（含云元数据 `169.254.169.254`）、`100.64.0.0/10`、`fc00::/7`、`fe80::/10`；`::ffff:a.b.c.d` 解包后再判 |
| SSRF 第二道 | `net.Dialer.Control` 在实际连接前用 sockaddr 的 IP 再校验一次（防 DNS rebinding） |
| 重定向 | `CheckRedirect` 对每一跳重跑上述校验；最多 5 跳；禁 https→http 降级 |
| 代理 | `Transport.Proxy = nil`，禁止走环境代理，否则校验被绕过 |
| 限额 | 单响应 8 MB（`io.LimitReader` 截断）、连接 5s / 首字节 10s / 总 30s、单源 QPS ≤ 1、全局并发 ≤ 4 |
| 编码 | 本包只透传 `Headers`/`Body`；调用方按「响应头 charset → 正文嗅探」用 `parser.DecodeBytes` 解码（见 §5） |
| 失败语义 | 单章最多重试 2 次；整本失败只记 `import_errors`，**不回滚已入库章节**；分页正文超上限记 warning 并保留已抓部分 |
| 隐私 | 书源 JSON 与抓到的正文都不进仓库；日志只记 host+path，不落 URL query（可能含 token） |

## 4. 规则引擎选型（数据说话）

抽样：4 个公开 GitHub 书源集合 + 1 个聚合文件，去重后 **69 个书源 / 1,705 条规则字符串**。
样本偏向复杂站与精选源，`@js` 占比按保守区间读（点估计 74%，区间 60%–75%）。

| 句法（按规则字符串主句法，互斥） | 条数 | 占比 |
|---|---|---|
| CSS 选择器（默认） | 1,236 | 72.5% |
| 嵌入 JS（`@js:`） | 179 | 10.5% |
| JSONPath（`@json:` / `$.`） | 136 | 8.0% |
| XPath（`@XPath:` / `//`） | 115 | 6.7% |
| 纯正则（`:` / `##`） | 39 | 2.3% |

含 `@js` 的规则字段共 **302/1,705 = 17.7%**；至少一条 JS 规则的**书源 51/69 = 73.9%**（区间 60–75%），零 JS 书源 26.1%。

| 方案 | 覆盖 | 二进制增量（实测） | 安全面 | 成本 |
|---|---|---|---|---|
| a) 纯声明式 | 1,403/1,705 = **82.3%** 字段；26.1% 书源零 JS | 0 | 无代码执行 | 低 |
| b) a + 受限表达式（管道 `replace/split/match/取值/变量`） | 估计 **93–97%** | 0（解释器，无引擎） | 无代码执行；正则用 RE2 防灾难回溯 | 中 |
| c) 嵌入 goja | 100% | **+9.03 MB**（1.57→10.60 MB） | 需自建沙箱（超时/内存/API 白名单） | 高 |
| c2) 嵌入 otto | 100%（ES5） | +3.80 MB（1.57→5.37 MB） | 同上 | 中高，但项目停更 |

**决策：采用 a + b，不嵌入 JS。** 理由按权重：
1. JS 规则的主体是字符串后处理与变量传递（`replace` 115、`match/exec` 103、`java.put/get` 107、`split` 58，
   真正二次请求 `java.ajax` 仅 25 条），管道表达式能吃下绝大部分，不必上引擎；
2. goja 让 16–18 MB 镜像 +56%，且真正的成本是「可审计沙箱」——Legado 从 JSON 规则迁到 JS 脚本后
   复杂度翻倍正是例子，我们不该重复这条路；
3. 含 JS 的书源**降级可用**：CSS 字段照常抓，JS 字段为空并记 warning，用户能看到残缺而不是静默错误。
4. 若将来必须上 JS：选 goja（维护活跃、ES2020）而非 otto（停更），并按「独立开关 + 单进程沙箱」另开 ADR。

## 5. 与现有解析管线复用

| 能力 | 现有实现 | 复用方式 |
|---|---|---|
| 编码探测 | `decodeStage` → `decodeByCodecs`（`internal/parser/parser.go:137`） | 新增导出 `parser.DecodeBytes`，内部转调同一套打分 |
| 噪声/换行/全角清洗 | `runFilters`（`parser.go:113`）+ filter 注册表（`registry.go:58`） | 新增导出 `parser.CleanText`，在线正文走同一条链 |
| 章节切分 | `Splitter` 层 | **不复用**：在线目录由 `ruleToc` 给定，禁止再猜章 |
| 元数据 | `Meta` 投票（`strategy.go:11`） | **不复用**：`ruleBookInfo` 直接给字段；缺失回落 `filename`/`stem` |
| 入库 | `store.SaveBook`（经 `importer` 的 `saveOne`） | 复用同一事务与幂等键（`bookSourceUrl` + 章节序号） |

## 6. 切片建议（依赖顺序）

1. **抓取执行器**：HTTP client（含 §3 全部校验）+ 编码/清洗复用（`DecodeBytes`/`CleanText`）。验收：SSRF 用例集（私网/回环/元数据/重定向/rebinding）+ 单站抓取。
2. **书源模型与存储**：`source_profiles` 键扩展 `kind=online` + `online` 子对象 + 校验（含 `unsupported_fields`）。依赖 1 的错误语义。
3. **规则执行器**：CSS/JSONPath/XPath/正则/模板 + 受限表达式管道；含 JS 的字段降级为 warning。验收：69 个样本源的字段解析回放（不含 JS 的部分）。
4. **抓取流水线**：搜索→详情→目录→正文→入库，追更只追加。依赖 1–3；复用 dry-run 报告形状。
5. **前端与导入导出**：书源 JSON 导入/导出、试跑、来源列表带「含 JS 规则」标记。

## 7. 本期不做

- **JS 规则**（`@js:`）——降级 + 可见 warning，不静默失败；
- 登录态/`loginUrl`/Cookie、请求头模板（`header` 里的动态签名）；
- 图片站/漫画源（`bookSourceType: 1`）、视频源；
- 书源市场与自动更新订阅（导入仍由用户显式上传 JSON）；
- 代理池与自定义 DNS。

## 8. 后果

- 含 JS 的书源（样本中 60–75%）**部分可用**：能出书名/目录，但正文可能拿不到——前端必须显示「该源含不支持规则」；
- 在线抓取默认关闭，开启后仍受 §3 约束，NAS 上不会出现无人值守的批量出网；
- 不引入 JS 引擎，镜像体积与「可审计」两条产品约束都保住；代价是高价值书源的覆盖率上限，需要时按 §4 第 4 条另开 ADR。

## 9. 相关

- [ADR-002 移除正文全文索引](0002-drop-fulltext-index.md)：在线抓取同样不建正文索引
- [ADR-005](0005-legado-import.md)：Legado 书源导入适配器（映射、类型过滤、@js 降级）
- [source-pluggable.md](../source-pluggable.md)：本地规则包（`kind=local`）与共用字段
- `/tmp/online-source-research.md`：抽样方法、原始频率表、goja/otto 实测命令（不入仓库）