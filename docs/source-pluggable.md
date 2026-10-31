# 来源可插拔：书源规则包

> 目标：新增一种「书的来路」（目录约定、命名格式、编码、切章写法、清洗与打标）只加一份 JSON，
> 不改 Go 代码、不发版。外部范式：Legado 声明式书源 JSON、Calibre 的 Python 插件与 recipe、
> Mihon 的独立扩展 APK、ReadYou 的 RSS 标准订阅——共同点不是「能装插件」，而是**规则与执行器分离**。

## 1. 结论先行

norvels 走**声明式规则包（Source Profile）+ 编译期策略注册表**，不做运行时插件，也不支持 JS。

| 选择 | 理由（为什么不是另一种） |
|---|---|
| 声明式 JSON，而非运行时 `.so` / 脚本 | 单二进制 + distroless + CGO-free（[architecture](architecture.md) §2）没有 dlopen 余地；引入 JS 引擎等于把「不可信规则」抬成「可执行代码」，沙箱、超时、内存上限都要自造。Legado 的 `@js:` 是它最重的部分，Tauri 版改成 JS 脚本后复杂度再翻一倍（Boa 引擎 + 宿主 API + 禁 DOM/箭头函数） |
| 复用 `internal/parser` 注册表，而非新建插件层 | 注册表 + `Options.Extra*` 已能承接「按来源定制」（[parser-architecture](parser-architecture.md) §3）。规则包只做编译：JSON → `Options` / `parser.Rule` / `store.TagRule`，零新接口 |
| 规则的编辑者是用户，不是开发者 | NAS 上自己改一份 JSON 就能适配新命名；Go 插件要重新编译发版，JS 脚本要一个可审计的沙箱。JSON 能校验、能试跑、能导出备份 |

**默认行为不变**：没有任何规则包时，导入走今天的内置策略，等价于当前实现。

## 2. 范式对照（只取能搬进本地库的那一条）

| 项目 | 机制 | 借鉴 | 不借鉴 |
|---|---|---|---|
| Legado | 书源 JSON：`bookSourceUrl` 唯一键、`bookSourceName/Group/Type`、`enabled/weight`、`searchUrl`/`exploreUrl`/`loginUrl`、`ruleSearch/ruleExplore/ruleBookInfo/ruleToc/ruleContent`；规则标记 `@css:` `@XPath:` `@Json:` `@js:`，正则 `##查找##替换###`，URL 变量 `{{key}}` `{{page}}` | 元字段（唯一键/分组/启停/权重）+ 规则块按「入口→详情→目录→正文」分段 + 导入导出 | `@js:`、`loginUrl`、站点请求头：norvels 不联网，这些字段没有作用对象 |
| Calibre | 插件是 ZIP + Python 类（`FileTypePlugin` 等）；recipe 是 `BasicNewsRecipe` 子类，声明字段 + `parse_index`/`preprocess_html`/`remove_tags` 钩子 | 「内置一大批 recipe + 只覆盖少数字段就能跑」的分层：先给默认，再按需改字段 | 进程内执行用户代码：它的信任模型是「用户自己装的插件」，与家庭共享口令下的导入来源不同 |
| Mihon / Tachiyomi | 扩展是独立 APK，主程序从扩展商店索引（`index.pb`）拉取列表、安装、按统一 Source 接口调用 | 来源清单与实现分离，扩展可整体禁用 | 安装包 + 索引分发：单二进制部署没有分发渠道，也无多进程隔离的必要 |
| ReadYou | 源 = RSS/Atom feed，靠标准协议 + 通用正文提取 | 能标准化就标准化；本地库对应的是扩展名/编码/章节标题的既定约定 | 不做站点规则：它把站点差异交给对方的标准格式 |

## 3. 规则包 schema 草案

存 `settings` 表的 `source_profiles` 键（JSON 数组，全量替换，与 `chapter_rules`/`tag_rules` 同形）。

| 字段 | 语义 | 说明 |
|---|---|---|
| `id` / `name` | 唯一键（同 `bookSourceUrl`）/ 展示名 | `id` 只允许 `[a-z0-9-]`，重名由校验拒绝 |
| `schemaVersion` | 规则包版本 | 只前进；不认识的版本**拒绝加载并报错**，不静默降级 |
| `enabled` / `priority` | 启停 / 优先级（大者优先） | 停用不影响已入库的书，只影响下一次导入 |
| `match` | `{ ext: ["txt"], pathGlob: "/books/示例/**", nameRegex: "^示例.*", zipNameRegex: "" }` | 匹配「一个来源路径」，多个条件**与** |
| `format` | 允许的容器格式 | 留空 = 取全局 `txt/epub/zip` |
| `decode` | `forceEncoding` | 直通 `Options.ForceEncoding` |
| `filters` | `disable: []` + `extra: [{ name, priority, pattern, replace }]` | `disable` 直通 `Options.DisableFilters`；`extra` 实例化成 `declFilter` |
| `meta` | `[{ name, weight, pattern, title, author, intro }]`（值为捕获组序号） | 实例化成 `declMeta`，权重决定投票顺位 |
| `split` | `forceSplitter` / `rules: [{ name, pattern }]` | `rules` 直通 `parser.Rule`（一行一判定，`^$` 锚定行首行尾） |
| `tags` | `[{ tag, keywords: [] }]` | 与现有词表打标同口径，命中写 `source=auto` |
| `limits` | `maxChapterChars` / `minChapters` | 直通两个已有选项 |

上限：规则包 ≤ 20 份、每份 `filters.extra + split.rules + meta` ≤ 30 条；单条正则 ≤ 200 字符照搬
`internal/api/handlers_settings.go:17` 的既有上限，不另立一套。

## 4. 与现有解析管线的映射

| 规则包字段 | 现有实现 | 动作 |
|---|---|---|
| `decode.forceEncoding` | `Options.ForceEncoding`（`internal/parser/parser.go:47`） | 直通 |
| `filters.disable` | `Options.DisableFilters`（`parser.go:44`）/ `runFilters`（`parser.go:113`） | 直通 |
| `filters.extra` | `Options.ExtraFilters`（`parser.go:50`） | 新增 `declFilter`（一个实现体按 JSON 实例化），Filter 接口不变 |
| `meta` | `Options.ExtraMetas`（`parser.go:51`）+ `mergeMeta` 逐字段投票（`strategy.go:101`） | 新增 `declMeta`，权重来自规则包 |
| `split.rules` | `parser.Rule`（`internal/parser/rules.go:21`）/ `Options.ChapterRules`（`internal/importer/importer.go:35`） | 直通；规则优先于内置切章（`parser.go:123`） |
| `split.forceSplitter` | `Options.ForceSplitter`（`parser.go:45`） | 直通；策略名必须在注册表里（`registry.go:68` `Splitters()`） |
| `tags` | `store.TagRule` + `SettingKeyTagRules`（`internal/store/settings.go:89`） | 直通（`importer.go:227` 已按书打分） |
| `limits` | `Options.MaxChapterChars`（`parser.go:41`）/ `importer.Options.MinChapters`（`importer.go:29`） | 直通 |
| `match` / `format` | `FormatOf`（`internal/importer/format.go:20`）、`isImportableCandidate`（`scan.go:99`） | **需新增**：扫描后按 `match` 选一份规则包，格式白名单从常量提升为「默认值 + 规则包覆盖」 |

选择语义：**显式指定 > `match` 命中且 `priority` 最高 > 内置默认**；同一文件**只应用一份规则包**，
不做多包叠加——叠加后无法解释「这本书为什么这样切章」，排障成本大于收益（Legado 也是「一次一个书源」）。

## 5. 能力边界（不支持什么比支持什么更重要）

| 能力 | 结论 | 理由 |
|---|---|---|
| 正则（行级、捕获组、替换） | **支持**，默认首选 | 现有 `split.rules` 与 `tag_rules` 已覆盖命名/切章/清洗的绝大多数写法 |
| `{{变量}}` 插值 | **支持** `{{sourceName}}` `{{ext}}` `{{profileId}}` | 只读上下文变量，不拼接 URL、不执行表达式 |
| CSS / XPath 选择器 | **不支持**（本地纯文本无 DOM） | 输入是字节流不是页面；硬加选择器等于为在线来源预埋第二套解析器 |
| JSONPath | **不支持**（同上） | 留到在线来源单独立项 |
| JS（`@js:` 类规则） | **不支持** | 无 JS 引擎；引入即失守「规则不可执行」这条安全边界 |
| 规则包内发起网络请求 | **不支持** | 破坏「无外部依赖、不联网」的产品前提；要做先出 ADR 定义白名单与 SSRF 口径 |

## 6. 导入 / 导出 / 校验 / 试跑

沿用 `docs/api/api-import.md` 的形状：全量替换（204）、校验错误 400、预检不建任务。

| 端点 | 语义 |
|---|---|
| `GET /api/import/source-profiles` | `{ "profiles": [...] }`，未配置返回空数组 |
| `PUT /api/import/source-profiles` | 全量替换；成功 204；`invalid_param`（超上限）/ `invalid_pattern`（正则编译失败，消息带第几条） |
| `POST /api/import/source-profiles/validate` | 只校验不落库，逐条返回 `{ index, id, ok, reason }` |
| `POST /api/import/source-profiles/{id}/dry-run` | 复用现有预检（`dryrun_scan.go`，前 200 个文件），报告里每本书带 `profileId` |
| `GET .../export` / `POST .../import` | 导出/导入同一份 JSON 数组；导入即校验 + 全量替换，失败不改库 |

前端沿用 `web/src/features/import/ImportPage.tsx` 左栏卡片位，新面板对齐 `ChapterRulesPanel.tsx`
（本地草稿 + 保存后整体替换 + 试跑走服务端 dry-run）。

## 7. 风险清单

| 风险 | 处置 |
|---|---|
| 规则包成为 SSRF 入口 | 本期不支持网络请求；在线来源前必须出 ADR：域名白名单、禁私网/回环/云元数据地址、禁跳转到私网 |
| 隐私（真实目录名/书名被写进规则包） | 仓库只放虚构示例，`pathGlob` 用 `/books/示例/**`；提交前跑 `scripts/privacy-scan.sh --all` |
| 规则版本演进与回滚 | `schemaVersion` 只前进；加载失败整包拒绝（不半生效）；回滚 = 导入上一次导出的 JSON |
| 性能（NAS、1.7 GB 库） | 导入开始时编译一次并缓存；正则条数与长度有上限；匹配是 O(文件 × 规则)，865 文件量级可忽略 |
| 两套真值漂移（内置正则 vs 规则包正则） | 规则包只写「本来源特有的写法」，通用写法改进仍落 `builtin_*.go`；先看内置清单再写规则包 |

## 8. 实现切片（按可独立验收拆分）

1. **规则包模型 + 存储 + 校验**：`source_profiles` 编解码、上限、`validate` 端点、前端面板。验收：单测（坏 JSON / 超限 / 坏正则报第几条）+ PUT/GET 契约。
2. **编译与接线**：JSON → `Options`/`Rule`/`TagRule`，新增 `declFilter`/`declMeta`；`importer.Options` 新增 `ProfileOf(path) *Profile`。验收：同目录两种命名各走一份规则包，一条 Gherkin。
3. **匹配选择与可见性**：`match` 打分、优先级、单包语义；`import_job_files` 记录 `profileId`，进度卡与失败清单展示。验收：优先级冲突用例 + 历史任务回看。
4. **导出 / 导入 / 试跑闭环**：JSON 往返、dry-run 报告带 `profileId`。验收：导出→改一个字段→导入→dry-run 结果变化可复现。
5. **（P2，需 ADR）在线来源**：HTTP 取回 + 域名白名单 + HTML/JSON 选择器子集，规则包内不执行 JS。不在本期承诺。

> 调研来源：[Legado 书源文档](https://gedoor.github.io/)（含 [规则语法与字段](https://docs.legadoteam.org/prompt/booksource.html)）、
> [Calibre 插件手册](https://manual.calibre-ebook.com/creating_plugins.html) 与 [recipe API](https://manual.calibre-ebook.com/news_recipe.html)、
> [Mihon 扩展商店](https://keiyoushi.github.io/docs/guides/getting-started)（`index.pb` 索引 + 独立扩展包）。
