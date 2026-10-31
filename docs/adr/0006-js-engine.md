# ADR-006：多类型规则 JSON 化与 JS 引擎引入

> 编号说明：ADR-005 已由「Legado 书源导入适配器」占用，本决策顺延为 0006。

> 决策日期 2026-10-31。触发：用户要求「导入规则、净化规则等不要写死在代码里，能像书源一样 JSON 导入」，
> 并放开「如果依赖 JS，可以考虑把 JS 引擎引进来」。原始调研见 `/tmp/rule-types-research.md`（脱敏，不入仓库）。

## 1. 调研结论：规则不止「书源」一类

公开分享站（aoaostar 仓库 + 一个书源分享站）实测到 6 类可导入规则，协议统一为
`legado://import/<type>?src=<url>`（旧版仍有 `yuedu://booksource/importonline?src=`）：

| 类型 | 协议名 | 落地 |
|---|---|---|
| 书源 | `bookSource` | `source.Profile(kind=online)`（已实现） |
| 净化规则 | `replaceRule` | **新增全局净化规则集**（见 §2） |
| TXT 目录规则 | `txtTocRule` | `split.rules`（结构与 `parser.Rule` 完全对应） |
| 订阅源 | `rssSource` | **本期不做**（语义是追更文章，不是本地库） |
| 朗读 / 排版 / 主题 | `httpTTS` / `readConfig` / `theme` | 不做（与本地书库目标无关） |

`src` 是 URL 本身（不是 base64）；导入器只取该 URL 的 JSON，**不执行链接里的任何内容**，
scheme 只允许 http/https。

## 2. 净化规则：映射到 Filter 层，不再写死

真实 replaceRule 字段：`name / group / order / pattern / replacement / isRegex / isEnabled / scopeTitle / scopeContent`。
存 `settings` 键 `replace_rules`（JSON 数组、全量替换，与 `chapter_rules`/`source_profiles` 同形），
编译为额外 `Filter` 注入 `parser.Options.ExtraFilters`：

- 只作用于「章节标题 / 正文」的范围由 `scopeTitle` / `scopeContent` 决定；
- 顺序按 `order`，与内置过滤器（bom/newline/noise/width）的相对顺序固定为「内置先行、自定义后置」；
- 停用（`isEnabled=false`）的规则保留但不生效，与 Legado 一致；
- **不写进代码**：内置 `cleaner.go` 的噪声词表保留作为兜底，新增规则一律走 JSON。

## 3. JS 引擎：引入 goja，默认关闭

**决策：引入 `github.com/dop251/goja`，作为可选能力，默认关闭。**

| 指标 | goja | otto | 依据 |
|---|---|---|---|
| 二进制增量 | **+9.03 MB**（1.57→10.60 MB） | +3.80 MB | 本机实测，`-s -w -trimpath` |
| 冷启动 `New()` | 108 µs | 未测 | 实测 |
| 单条规则执行 | 15.9 µs（1000 次共 15.9 ms） | 未测 | 实测 |
| 死循环中断 | **≈1.16 ms**（`vm.Interrupt`） | 未测 | 实测 |
| 1000 次后堆占用 | 535 KiB | 未测 | 实测 |
| 维护状态 | 活跃（ES2020） | 停更（v0.5.1，2022） | 仓库状态 |
| CGO | 纯 Go | 纯 Go | 均满足 `CGO_ENABLED=0` |

选 goja 不选 otto：多 5.2 MB 换来活跃维护与可中断执行；镜像 16–18 MB → 约 25–27 MB，仍在 50 MB 目标内。
不选「自研 JS 子集」：`@js:` 规则本身就是任意 JS（含正则、闭包、数组方法），做子集等于做一个残缺引擎。

## 4. 沙箱设计

| 面 | 做法 |
|---|---|
| 执行单位 | 一条规则一次 `vm.RunString`；不复用 VM（避免全局状态串扰） |
| 超时 | 每条 **200 ms** 硬超时，到点 `vm.Interrupt("timeout")`（实测 1.16 ms 内返回） |
| 脚本体积 | 单条规则 ≤ 8 KB，超限拒绝编译并记 `js_too_large` |
| 调用栈 | `SetMaxCallStackSize(2000)`，防深递归 |
| 内存 | goja 无堆配额；用「脚本体积 + 超时 + 栈深」三道闸，进程级由 Go runtime 兜底（不做每 VM 记账） |
| API 白名单 | `result`（当前值）、`java.put/get`（规则间变量）、`java.base64`、`java.md5`、`java.timeFormat`、`java.encodeURI`、`JSON.parse/stringify`、`String/Array/RegExp` 基础方法；`console.log` 只进内存缓冲，不外写 |
| 禁用 | `require`、`eval`、`Function` 构造器（运行前从全局删除）、任何文件/网络/进程 Host API（不注入） |
| 逃逸面 | 不注入 `os`/`net`/`syscall`；不把 Go 函数暴露成可反射对象；`Interrupt` 只由宿主 goroutine 调用 |

## 5. 开关与降级（对齐现有 degradedFields）

| 场景 | 行为 |
|---|---|
| 默认（未开启） | JS 规则字段 → 编译期标记 `degraded`，`reason = js_disabled`；hard/soft 分类沿用 0004 §5（bookList/chapterList/content/searchUrl/exploreUrl 为 hard） |
| 全局开启 `NORVELS_JS=on` | 允许执行；书源级 `online.allowJS` 可再收紧（默认跟随全局） |
| 开启但执行失败/超时 | 该字段返回空 + 运行期 warning `js_error:<block>.<field>`；**不重试、不重跑整本** |
| 未开启的旧书源 | 行为完全不变，仍按 CSS/XPath/JSONPath/管道求值 |

`degradedFields` 新增 `reason` 取值 `js_disabled`（未开启）与 `js_error`（执行失败），与 `js_unsupported` 并列；
frontend 只需要按 reason 展示不同文案，字段与 hard/soft 语义不变。

## 6. 与 ruleexec 的关系

- JS **只用于单条规则求值**，不替代 CSS/XPath/JSONPath/正则/管道；
- 求值顺序：**声明式优先**——规则以 `@js:` 开头才走 JS；纯声明式规则永远不进引擎；
- 开关关闭时 `@js:` 规则不执行、不报错、只记降级，整源仍可用（hard 字段除外）；
- `internal/ruleexec` 增加 `Evaluator` 接口（`Eval(ctx, script) (string, error)`），由 `internal/jsruntime` 实现；
  ruleexec 不直接 import goja，保持叶子包与可替换性。

## 7. 切片

1. **净化规则集**：`replace_rules` 存储 + 校验 + 编译成 Filter + 导入/导出（不依赖 JS）。
2. **TXT 规则集**：`txt_rules` 存储 + 编译成 `parser.Rule`（不依赖 JS）。
3. **jsruntime**：goja 包装（超时/中断/白名单/栈深），独立叶子包 + 单测（含中断用例）。
4. **接线**：ruleexec 的 `Evaluator` 注入；开关、降级 `reason`、前端文案。

## 8. 不做

- 订阅源（rssSource）与朗读/排版/主题；
- 书源 `loginUrl`/Cookie 登录态；
- 每 VM 内存配额（goja 不提供，硬做等于改引擎）；
- 在 JS 里发起网络请求（宿主管制抓取，JS 不碰网络）。

## 9. 后果

- 默认关闭时体积与行为与今天完全一致（goja 代码虽编译进二进制，但不初始化）；
- 开启后镜像约 +9 MB，换来书源 73.9% 与净化规则的核心表达方式可用；
- 多一类可导入规则（净化/TXT），导入入口统一为「JSON 数组 + 全量替换」，与书源同形。

## 10. 相关

- [ADR-004 在线书源与规则引擎选型](0004-online-book-source.md)：声明式优先、degradedFields 语义
- [source-pluggable.md](../source-pluggable.md)：`kind=local` 规则包
- `/tmp/rule-types-research.md`：字段表、协议、JS 实测数据（不入仓库）