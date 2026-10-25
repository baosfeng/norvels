# 解析器架构：可插拔策略管线

> 目标：面对未来的多来源、多格式语料，新增一种编码 / 一种广告 / 一种章节写法时，
> 只需**新增一个文件并注册**，不改动已有代码。

## 1. 分层

文件先按扩展名进**格式容器层**：`.txt` 直接得到字节流；`.epub` 交给 `internal/ebook`
（zip → container.xml → OPF → spine，并按 nav.xhtml / toc.ncx 解析目录层级映射成卷）
解出纯文本章节、元数据与封面。容器层以下
完全共用同一条管线——EPUB 的 XHTML 同样走编码兜底、噪声清洗与（必要时）切章策略。

```
raw bytes（txt）或 ebook.Book（epub 已解包）
   │
   ├─ Codec 层      编码候选解码（utf-8 / gb18030 / utf-16 / big5…）→ 选最高分
   │
   ├─ Filter 链     有序文本变换（剥 BOM / 换行归一 / 噪声清洗 / 全角半角…）
   │
   ├─ Meta 层       元数据抽取（文件名 / 正文头部 / 章节标题反推），逐字段投票
   │
   ├─ Splitter      切章策略：Detect 打分 → 选最高分策略执行
   │
   └─ PostProcess   空章清理 / 超长章再切 / 卷重排
```

各层只依赖 `Doc`，层间不直接互调；执行顺序与启停由 `Pipeline` 决定。

**降级语义（必须遵守）**：任何单个策略 panic 或返回 error，都只是记一条 warning
并跳到下一个候选，绝不把整篇解析带崩。切章层尤其如此——选中的策略失败时回退到
`unmarked` 兜底，保证「至少能入库」。实现上由 `guard()` 统一包 recover。

## 2. 契约（internal/parser/strategy.go）

```go
// Doc 是一次解析的载体，各阶段只读写自己关心的字段。
type Doc struct {
    Name     string               // 原始文件名（含扩展名）
    Raw      []byte               // 原始字节
    Text     string               // 解码后全文
    Encoding string               // 选中的编码名
    EncScore float64              // 编码置信度
    Meta     Meta                 // 书名 / 作者 / 简介
    Volumes  []model.ParsedVolume
    Chapters []model.ParsedChapter
    Warnings []string
    Stats    map[string]any       // 各阶段诊断（策略命中情况、耗时）
}

type Codec interface {
    Name() string
    // Try 返回解码文本与置信分（0 表示该编码不适用）。
    Try(raw []byte) (text string, score float64)
}

type Filter interface {
    Name() string
    Priority() int      // 升序执行；同优先级按注册顺序
    Apply(d *Doc) error // 就地修改 Text / Warnings
}

type MetaExtractor interface {
    Name() string
    Weight() int                       // 策略本身的可信度（越大越可信）
    Extract(d *Doc) (Meta, int)        // 抽到的字段 + 置信分 0-100；0 表示放弃
}

type Splitter interface {
    Name() string
    Detect(d *Doc) float64             // 本文本适配度 0-1；0 表示不认领
    Split(d *Doc) error                // 结果写入 d.Volumes / d.Chapters / d.Warnings
}
```

`Meta` 是逐字段投票的载体：`Extract` 只填它有把握的字段，分数字段独立比较，
所以「文件名给书名、正文头部给作者」可以同时生效。

## 3. 注册与配置（internal/parser/registry.go）

```go
func RegisterCodec(c Codec)
func RegisterFilter(f Filter)
func RegisterMeta(m MetaExtractor)
func RegisterSplitter(s Splitter)

func Codecs() []Codec
func Filters() []Filter
func Metas() []MetaExtractor
func Splitters() []Splitter
```

- 内置策略在包初始化时注册：`registry.go` 的 `init()` 调四个 `registerBuiltin*`，
  各自定义在对应的 `builtin_*.go` 里（一屏可见本层支持哪些格式）
- 文件布局：`strategy.go`（契约 + `guard` 降级）· `registry.go`（注册表）·
  `builtin_codec.go` / `builtin_filter.go` / `builtin_meta.go` / `builtin_splitter.go`（内置策略）·
  `postprocess.go`（超长章再切）· `heading*.go` / `split_*.go` / `cleaner.go` /
  `meta_filename*.go` / `meta_preamble.go` / `normalize.go`（各层实现体）
- 第三方 / 灰度策略通过 `Options.ExtraFilters` / `ExtraSplitters` 注入，不改全局
- `Options` 增加启停与强制项，便于排障与按来源定制：

| 字段 | 作用 |
|---|---|
| `DisableFilters []string` | 按名关闭过滤器（排障 / 特殊语料） |
| `ForceSplitter string` | 跳过 Detect，强制指定切章策略 |
| `ForceEncoding string` | 跳过打分，强制指定编码 |
| `ExtraFilters/ExtraMetas/ExtraSplitters` | 单次解析注入策略，不污染全局注册表 |
| `MaxChapterChars` | 单章字符上限，超过则按段落边界再切（默认 30000，0 关闭） |

## 4. 内置策略清单

| 层 | 名称 | 说明 |
|---|---|---|
| Codec | `utf8` `utf8-bom` `gb18030` `utf16le` `utf16be` `big5` | 逐个 Try，按 CJK 命中率 − U+FFFD 惩罚打分 |
| Filter | `bom`（10） | 剥 UTF-8 / UTF-16 BOM |
| Filter | `newline`（20） | \r\n / \r → \n，去零宽字符 |
| Filter | `noise`（30） | 书源广告行 / 群号 / 网址清洗（保守，仅边界与短行） |
| Filter | `width`（40） | 全角数字 / 字母归一（影响章节号识别） |
| Meta | `filename`（weight 80） | 文件名规则优先 |
| Meta | `preamble`（weight 60） | 正文头部元信息块 |
| Meta | `heading-reverse`（weight 40） | 由章节标题反推（头部信息缺失时兜底） |
| Meta | `stem`（weight 10） | 兜底：文件名主干即书名，保证书名永不落空 |
| Splitter | `di-unit` | 「第N章/节/回/篇/话/卷/部」 |
| Splitter | `special` | 序章 / 楔子 / 引子 / 尾声 / 番外 |
| Splitter | `numeric` | 纯数字行 / `1、标题` |
| Splitter | `en-chapter` | `Chapter N` |
| Splitter | `unmarked` | 兜底：无标记时按长度等分（Detect 恒为最低正分） |

> `internal/ebook` 不注册进上面的策略表：它输出的是**已结构化**的 `Book`（spine 即章节），
> 由 `importer` 转成 `model.ParsedBook`。只有「整本塞在一个 XHTML 里」的书才把全文交回
> Splitter 切章。新增其它 EPUB 类容器（如 DOCX）时复用同一条「解包 → 结构化」路径。

**卷与合集（P1）**：EPUB 的目录层级（`nav.xhtml` 优先、`toc.ncx` 兜底）只提供章标题与卷归属，
章序与章数一律以 spine 为准；`Chapter.VolumeIdx` 是 1 起（0=未分卷），由 importer 转成 model 的
0 起（-1=未分卷）。txt 合集由 `importer` 消费 `parser.SplitSegments`（章号重置点，每段 ≥3 章）
产出多本 `ParsedBook`，第 N 段来源路径标 `#N`、哈希标 `:N`。

## 5. 扩展方式（新增格式怎么做）

| 场景 | 动作 |
|---|---|
| 新编码（如 GBK 变体 / Shift-JIS） | 实现 `Codec` + `RegisterCodec`，一个文件 |
| 新广告 / 水印形态 | 实现 `Filter`，给一个合适 Priority，一个文件 |
| 新书名写法 | 实现 `MetaExtractor`，一个文件 |
| 新章节写法（如 markdown `## 三`、`【第三章】`） | 实现 `Splitter`，`Detect` 给出适配度，一个文件 |
| 某目录要特殊处理 | 用 `Options.Extra*` 注入，不改全局 |

## 6. 验证口径

- `go test ./internal/parser/ -cover`：parser 包覆盖率 ≥ 85%（当前 85.8%）
- 每个内置策略至少一条定向测试（正常 + 边界 + 放弃/降级路径）
- 策略叠加的回归用例：同一 fixture 在不同 `ForceSplitter` / `ForceEncoding` 下结果可预期
- 真实语料抽检仍走 `TestSampleRealCorpus`（`NORVELS_SAMPLE_DIR` 环境变量，默认跳过）

## 7. 兼容性

- `New(Options)` / `ParseFile` / `ParseBytes` 签名不变，现有调用方（importer / api）零改动
- 旧实现作为内置策略的**实现体**保留，先包装再逐步拆细：不在一次改动里重写算法，
  避免把已通过的语义测试一起推翻
