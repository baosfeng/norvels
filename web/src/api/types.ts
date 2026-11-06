/** 后端 REST API 契约（docs/api.md v1）的 TypeScript 映射。字段与后端一一对应，不做改名。 */

export type BookStatus = "parsed" | "pending" | "failed";

/**
 * 连载状态（P0）：与解析状态 `status` 是两回事，后者说的是入库处理进度。
 * `""` = 未知（存量书回填值，不猜）。
 */
export type SerialStatus = "" | "ongoing" | "completed";

/**
 * 阅读状态（P1）：`""` = 未标记（存量书不回填，前端不猜）。
 * 与解析状态 `status`、连载状态 `serialStatus` 互不覆盖。
 */
export type ReadStatus = "" | "want" | "reading" | "done";

/** 导入时从文件带出的附加元数据；目录导入的老书这些字段缺省，前端一律按「没有」处理。 */
export interface BookMeta {
  language?: string;
  publisher?: string;
  isbn?: string;
  pubDate?: string;
  series?: string;
  seriesIndex?: number;
  /** 这本书是否存了封面：为真才去取图，否则直接用首字占位 */
  hasCover?: boolean;
  /** 来源格式：txt / epub */
  format?: string;
  /** 大分类（0015）：导入时按来源目录第一级写入，空串 = 未分类 */
  categoryL1?: string;
  /** 小分类（0015）：来源目录第二级，空串 = 该大分类下未细分 */
  categoryL2?: string;
}

/** GET /api/categories 的响应：大分类 → 小分类 → 书数（两级，最多两层）。 */
export interface CategoryLeaf {
  l2: string;
  count: number;
}

export interface CategoryNode {
  l1: string;
  count: number;
  items: CategoryLeaf[];
}

export interface CategoryTree {
  items: CategoryNode[];
  total: number;
}

/** PATCH /api/books/{id} 的请求体：任意子集；tags 与 authors 是全量替换。 */
export interface BookMetaPatch {
  title?: string;
  /** 主作者。改作者请用 authors：后端 authors 优先，两者同时出现时 author 被忽略。 */
  author?: string;
  /** 有序作者列表，全量替换；空数组后端 400（至少一个作者）。 */
  authors?: string[];
  /** 简介；传 "" 即清空。 */
  intro?: string;
  tags?: string[];
  /** 系列名；传 "" 即清空。 */
  series?: string;
  /** 系列序号；传 0 即清空。 */
  seriesIndex?: number;
  /** 连载状态；`""` 即未知。其它取值后端 400 `invalid_param`。 */
  serialStatus?: SerialStatus;
  /** 阅读状态；`""` 即未标记。其它取值后端 400 `invalid_param`。 */
  readStatus?: ReadStatus;
  /** 出版社；传 "" 即清空。 */
  publisher?: string;
  /** 出版日期 YYYY-MM-DD；传 "" 即清空，格式不对后端 400 invalid_param。 */
  pubDate?: string;
  /** ISBN；传 "" 即清空。 */
  isbn?: string;
  /** 语言；传 "" 即清空。 */
  language?: string;
  /** 评分 0–5：0 = 未评（不参与 rating 筛选与 sort=rating），越界后端 400 invalid_param。 */
  rating?: number;
  /** 大分类；传 "" 即清空（落到「未分类」）。 */
  categoryL1?: string;
  /** 小分类；传 "" 即清空。 */
  categoryL2?: string;
}

export interface UploadAccepted {
  name: string;
  format: string;
}

/** 逐文件拒绝原因：code 给程序分支用，message 是可直接展示的人话。 */
export interface UploadRejected {
  name: string;
  code: string;
  message: string;
}

/** POST /api/import/upload 的 202 响应：部分文件被拒也照样建任务。 */
export interface UploadResult {
  jobId: number;
  accepted: UploadAccepted[];
  rejected: UploadRejected[];
}

export interface BookSummary extends BookMeta {
  id: number;
  title: string;
  authorId: number;
  authorName: string;
  chapterCount: number;
  wordCount: number;
  volumeCount: number;
  intro: string;
  status: BookStatus;
  /** 阅读状态；后端未升级时缺省，前端一律按 `""`（未标记）处理。 */
  readStatus?: ReadStatus;
  /** 连载状态；列表也返回（管理页的筛选与列都用它），缺省按 `""`（未知）。 */
  serialStatus?: SerialStatus;
  /** 本书书签总数；缺省时按「还不知道」处理，不展示。 */
  bookmarkCount?: number;
  /** 评分 0–5；缺省按 0（未评）处理，与后端「0 = 未评」同一口径。 */
  rating?: number;
  /** 追更新增章数；缺省按 0。只有追更链路累加，读完最后一章由 ack 清零。 */
  newChapters?: number;
  /** 最后一章入库时间（UTC ISO-8601）；缺省按 `""`（还没有章节）处理。 */
  lastChapterAt?: string;
  importedAt: string;
  updatedAt: string;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

/** 连载状态筛选值：none 专门筛「未知」（后端用同名字段承接）。 */
export type SerialStatusFilter = Exclude<SerialStatus, ""> | "none";

/** POST /api/books/{id}/chapters/{cid}/split 的响应：拆完后的章数。 */
export interface SplitResult {
  chapters: number;
}

/** POST /api/books/{id}/chapters/merge 的响应：合并后全书剩多少章。 */
export interface MergeResult {
  chapters: number;
}

/** POST /api/books/{id}/chapters/renumber 的响应：改了多少章的名字。 */
export interface RenumberResult {
  renamed: number;
}

/**
 * renumber 的请求体：全书，或只改某一卷。
 * 跨卷 / 不连续 / 单章合并会被后端 409 not_mergeable 挡下（那一项只属于 merge）。
 */
export type RenumberBody = { scope: "book" } | { scope: "volume"; volumeId: number };

export interface ChapterBrief {
  id: number;
  idx: number;
  title: string;
  volumeId: number;
  wordCount: number;
}

export interface Volume {
  id: number;
  idx: number;
  title: string;
  chapters: ChapterBrief[];
}

export interface BookDetail extends BookSummary {
  /** 有序作者列表；存量书按主作者兜底，所以通常至少一项。authorName 仍是主作者。 */
  authors: string[];
  /** 最后一章（idx 最大的章节）的标题；书还没有章节时缺省 */
  lastChapterTitle?: string;
  /** 最后一章的章序（从 0 起）；书还没有章节时缺省 */
  lastChapterIdx?: number;
  encoding?: string;
  sourceName?: string;
  fileSize?: number;
  /**
   * 当前标签（全量）。后端返回就用来回填编辑表单；
   * 未返回时表单标签留空，且不提交标签字段，避免误清空已有标签。
   */
  tags?: string[];
  volumes: Volume[];
}

/**
 * 书签（段落级）：`paraIndex` 是正文按 `\n` 切段后的段落下标，与后端同一口径。
 * 列表按 created_at 倒序返回。
 */
export interface Bookmark {
  id: number;
  chapterId: number;
  chapterIdx: number;
  chapterTitle: string;
  paraIndex: number;
  /** 默认取该段前 30 字（去空白），可留空 */
  label: string;
  createdAt: string;
}

export interface ChapterDetail {
  bookId: number;
  bookTitle: string;
  id: number;
  idx: number;
  title: string;
  content: string;
  wordCount: number;
  prevId: number;
  nextId: number;
}

export interface AuthorSummary {
  id: number;
  name: string;
  bookCount: number;
  wordCount: number;
}

export interface AuthorDetail extends AuthorSummary {
  books: BookSummary[];
}

export interface SearchResult {
  q: string;
  /** 书名与作者命中——这也是**唯一**的检索范围（章节标题检索已下线，后端不再返回 chapters 字段） */
  books: Page<BookSummary>;
  /** 后端每次都会返回；可读提示语，为空串表示本次没有要额外说明的事 */
  notice?: string;
}

export interface ImportJob {
  id: number;
  sourceDir: string;
  /** completed_with_errors = 部分文件失败（迁移 0012 起由逐文件终态算出来） */
  status: "queued" | "running" | "completed" | "completed_with_errors" | "failed" | "canceled";
  total: number;
  imported: number;
  skipped: number;
  failed: number;
  startedAt: string | null;
  finishedAt: string | null;
  message: string;
  errors?: { filePath: string; reason: string }[];
  /** 失败文件数（= import_job_files 里 failed 的条数）；后端未升级时缺省 */
  failedCount?: number;
}

/** 导入任务的逐文件终态；重试按 `source` 重放。 */
export type ImportFileStatus = "imported" | "failed" | "skipped" | "unsupported" | "canceled";

export interface ImportJobFile {
  id: number;
  /** 可重放源：目录文件是绝对路径；zip 条目是 `<zip 绝对路径>:<包内路径>` */
  source: string;
  /** 只用于展示的路径（可能是相对路径或伪路径） */
  displayPath: string;
  status: ImportFileStatus;
  reason: string;
  /** 导入成功的书 id；失败 / 跳过为 0 */
  bookId: number;
  updatedAt: string;
}

export interface ImportJobFiles {
  files: ImportJobFile[];
  /** 超过 1000 条时后端只返回前面部分 */
  truncated?: boolean;
}

/** 只预检（dry-run）报告：不建任务、不占锁、不写库，口径与真实导入一致。 */
export interface DryRunReport {
  /** 目录条目 > 200 时只解析前 200 条 */
  truncated: boolean;
  files: {
    total: number;
    willImport: number;
    duplicate: number;
    unsupported: number;
    failed: number;
  };
  books: { new: number; duplicate: number; split: number };
  /** 最多 20 条，按扫描顺序 */
  sample: DryRunSample[];
  /** 最多 50 条 */
  errors?: { source: string; reason: string }[];
  /** 规则包相关的告警（例如某份规则包没匹配上任何文件），面向用户的一句话 */
  profileWarnings?: string[];
}

export interface DryRunSample {
  source: string;
  title: string;
  chapters: number;
  words: number;
  duplicate: boolean;
  reason: string;
  /** 这本书实际生效的规则包 id；空串 = 走内置策略（显式指定或 match 命中）。 */
  profileId?: string;
}

/** 词表规则：命中关键词就给书打这个标签（导入时执行，只增不删，不覆盖手动标签）。 */
export interface TagRule {
  tag: string;
  keywords: string[];
}

export interface TagRules {
  rules: TagRule[];
}

/**
 * 服务端能力清单（GET /api/settings/capabilities）：上限与策略名单的唯一来源。
 *
 * 只读下发、不接受写入——上限是护栏，可配置等于把 DoS 开关交给一份 JSON。
 * 前端不复制这些数字：读不到时按「不限制 + 服务端仍会校验」保守放行。
 */
export interface CapabilityFilter {
  name: string;
  priority: number;
}

export interface CapabilityMeta {
  name: string;
  weight: number;
}

/** 章节规则上限。 */
export interface ChapterRuleLimit {
  maxCount: number;
  maxPatternRunes: number;
  maxNameRunes: number;
}

/** 词表打标规则上限。 */
export interface TagRuleLimit {
  maxCount: number;
  maxTagRunes: number;
  maxKeywords: number;
  maxKeywordRunes: number;
}

/** 书源规则包上限（切片 4 的面板用，这里先与后端对齐）。 */
export interface SourceProfileLimit {
  maxCount: number;
  maxRulesPerProfile: number;
  maxPatternRunes: number;
  maxNameRunes: number;
}

/** 本进程生效的上传上限（随服务端 Config 注入，0 值已由后端回落默认）。 */
export interface UploadLimit {
  maxFileBytes: number;
  maxTotalBytes: number;
  maxFiles: number;
}

/** 导入线程的规模参数与解析默认值。 */
export interface ImportLimit {
  maxWorkers: number;
  defaultMaxChapterChars: number;
  defaultFallbackChunkChars: number;
}

/** 预检（dry-run）的规模上限：候选文件数、样例条数、错误条数。 */
export interface DryRunLimit {
  maxFiles: number;
  samples: number;
  errors: number;
}

/** 规则包的来源类型：本地文件 / 在线书源（本期只校验结构，不发起抓取）。 */
export type SourceProfileKind = "local" | "online";

/** match：哪些来源路径归这份规则包；条件之间是「与」，一份文件只应用一份规则包。 */
export interface SourceMatch {
  ext?: string[];
  pathGlob?: string;
  nameRegex?: string;
  zipNameRegex?: string;
}

export interface SourceDecode {
  forceEncoding?: string;
}

/** 一条声明式清洗规则（filters.extra）。 */
export interface SourceFilterRule {
  name: string;
  priority: number;
  pattern: string;
  replace: string;
}

export interface SourceFilters {
  /** 关闭内置过滤器（名字取 capabilities.filters） */
  disable?: string[];
  extra?: SourceFilterRule[];
}

/** 一条元数据抽取规则：pattern 的捕获组序号写到 title/author/intro（0 = 整段匹配）。 */
export interface SourceMetaRule {
  name: string;
  weight: number;
  pattern: string;
  title: number;
  author: number;
  intro: number;
}

/** 一条行级切章规则（直通 parser.Rule）：^$ 锚定整行。 */
export interface SourceSplitRule {
  name: string;
  pattern: string;
}

export interface SourceSplit {
  /** 内置切章策略名（取 capabilities.splitters）；空串 = 用 Detect 打分选 */
  forceSplitter?: string;
  rules?: SourceSplitRule[];
}

export interface SourceTagRule {
  tag: string;
  keywords: string[];
}

export interface SourceLimits {
  maxChapterChars: number;
  minChapters: number;
}

/** 在线书源段：本期只做结构校验，不发起任何网络请求。 */
export interface SourceOnline {
  bookSourceUrl: string;
  group?: string;
  weight?: number;
  ruleSearch?: Record<string, string>;
  ruleBookInfo?: Record<string, string>;
  ruleToc?: Record<string, string>;
  ruleContent?: Record<string, string>;
}

/** 一份书源规则包（与后端 source.Profile 同形，字段语义见 docs/source-pluggable.md §3）。 */
export interface SourceProfile {
  id: string;
  name: string;
  schemaVersion: number;
  enabled: boolean;
  priority: number;
  kind: SourceProfileKind;
  match: SourceMatch;
  /** 允许的容器格式；留空 = 取全局扩展名清单 */
  format?: string[];
  decode: SourceDecode;
  filters: SourceFilters;
  meta?: SourceMetaRule[];
  split: SourceSplit;
  tags?: SourceTagRule[];
  limits: SourceLimits;
  online?: SourceOnline;
}

export interface SourceProfiles {
  profiles: SourceProfile[];
}

/** Legado 条目类型（预览表按它显示「书源 / 净化 / TXT 规则 / 未知」）。 */
export type SourceImportType = "book_source" | "replace_rule" | "txt_toc_rule" | "unknown";

/**
 * 一条输入的导入结果状态。
 * ok / partial 可入库（partial 带降级标记）；unusable 的关键字段依赖 @js，抓取必失败；
 * invalid / unsupported_type 明确拒绝；recognized / unknown_type 本期不落库。
 */
export type SourceImportStatus =
  | "ok"
  | "partial"
  | "unusable"
  | "invalid"
  | "unsupported_type"
  | "recognized"
  | "unknown_type";

/** POST /api/import/source-profiles/import 的逐条结果。 */
export interface SourceImportIssue {
  index: number;
  type: SourceImportType | string;
  id: string;
  name: string;
  status: SourceImportStatus | string;
  degradedFields: string[];
  unsupportedFields: string[];
  reason: string;
}

/** apply=true 时被跳过的条目（新语义：跳过不可用、只落可用）。 */
export interface SourceImportSkipped {
  index: number;
  id: string;
  name: string;
  type: string;
  status: string;
  reason: string;
}

/** 预览 / 应用书源的响应。 */
export interface SourceImportResult {
  applied: number;
  overwritten: number;
  results: SourceImportIssue[];
  /** 新语义下才有：本次被跳过的条目 */
  skipped?: SourceImportSkipped[];
  /** 输入形态：text / url / legado / yuedu（由后端取回层判定） */
  via?: string;
}

/** POST /api/import/source-profiles/validate 的逐份结果。 */
export interface SourceProfileIssue {
  index: number;
  id: string;
  ok: boolean;
  reason: string;
}

export interface SourceProfileValidateResult {
  results: SourceProfileIssue[];
}

export interface Capabilities {
  splitters: string[];
  codecs: string[];
  /** 按 priority 升序（= 执行顺序） */
  filters: CapabilityFilter[];
  /** 按 weight 降序（= 投票顺位） */
  metas: CapabilityMeta[];
  /** 可导入的扩展名（小写、不带点） */
  extensions: string[];
  limits: {
    chapterRules: ChapterRuleLimit;
    tagRules: TagRuleLimit;
    sourceProfiles: SourceProfileLimit;
    upload: UploadLimit;
    import: ImportLimit;
    dryRun: DryRunLimit;
    /** 分页上限（列表一次最多取多少行）；旧服务端不下发，取用时给默认值 */
    pagination?: { maxSize: number };
  };
  /**
   * 推荐流运行时能力：`enabled=false` 表示推荐库打不开，整块不该渲染。
   * 标为可选是因为旧服务端（与只关心导入域的测试夹具）不下发这一组，取用时必须给默认值。
   */
  recommend?: {
    enabled: boolean;
    tabs: { id: string; label: string }[];
    defaultLimit: number;
    maxLimit: number;
  };
  /** 埋点契约：批量、字段长度、合法 kind（kind 清单唯一事实源在服务端）；同上可选 */
  events?: {
    maxBatch: number;
    maxFieldLength: number;
    maxExtraLength: number;
    maxTags: number;
    kinds: string[];
  };
  /** AI 能力可用性（有 key 才有）；模型名仅供展示；同上可选 */
  ai?: {
    reviewAvailable: boolean;
    curationAvailable: boolean;
    model?: string;
  };
}

export interface Stats {
  books: number;
  chapters: number;
  authors: number;
  words: number;
  dbBytes: number;
  lastImportAt: string | null;
  /** 体积构成：正文部分**压缩后**的实际占用（用户真正占的磁盘） */
  textBytes?: number;
  /**
   * 正文压缩前的逻辑字节（原始文字有多少）。
   * 与 textBytes 之比就是压缩率，前端只用它算「省了多少」，不参与体积构成的百分比。
   */
  textBytesRaw?: number;
  indexBytes?: number;
  otherBytes?: number;
  /** 体积构成三段之和恒等于 dbBytes（后端按实际文件大小兜底过） */
  compositionEstimated?: boolean;
  /** 预计算表正在刷新：数值可用但可能不是最新，前端据此提示「数据更新中」 */
  stale?: boolean;
}

export interface Bucket {
  label: string;
  count: number;
}

export interface HealthReport {
  totalBooks: number;
  thinBooks: number;
  missingAuthor: number;
  missingIntro: number;
  hugeChapters: number;
  chapterBuckets: Bucket[];
  chapterLengthBuckets: Bucket[];
  warnings: string[];
  /**
   * 阅读进度三态（按书聚合，后端直接算好）。
   * 契约见 leader 2026-10-25 通知；后端未上线时该字段为 undefined，前端回退到
   * 「/reading/progress 最近 50 条 + 总书数」的估算，并标注口径。
   */
  readingBuckets?: ReadingBuckets;
  /** 同上：统计表刷新期间为 true */
  stale?: boolean;
}

export interface ReadingBuckets {
  unread: number;
  reading: number;
  finished: number;
}

/**
 * 库健康问题的可下钻类型：与 /api/health/issues 的 kind 一一对应。
 * 前三个是书级问题，huge_chapter 是章节级问题。
 */
export type HealthIssueKind = "missing_author" | "missing_intro" | "thin_book" | "huge_chapter";

/**
 * 健康明细里的一条。书级问题只填书字段；章节级问题（超长章节）另外带
 * chapterId / chapterIdx / chapterTitle，此时 wordCount 是**这一章**的字数。
 */
export interface HealthIssueItem {
  bookId: number;
  title: string;
  author: string;
  chapterCount: number;
  wordCount: number;
  chapterId?: number;
  /** 0 号章也是有效下标（后端用指针区分「没有」与「第 0 章」） */
  chapterIdx?: number;
  chapterTitle?: string;
}

export interface HealthIssues {
  kind: HealthIssueKind;
  total: number;
  page: number;
  size: number;
  items: HealthIssueItem[];
}

export interface ReadingProgress {
  bookId: number;
  bookTitle: string;
  chapterId: number;
  chapterIdx: number;
  chapterTitle: string;
  percent: number;
  /** 章内段落下标（与书签 paraIndex 同一口径）；缺省按 0。 */
  paraIndex?: number;
  updatedAt: string;
}

export interface ImportTrendPoint {
  date: string;
  books: number;
  chapters: number;
}
export interface TagSummary {
  id: number;
  name: string;
  bookCount: number;
}

/** 自定义章节识别规则（全局）：导入时按顺序尝试，命中即作为章节边界。 */
export interface ChapterRule {
  name: string;
  /** 正则源码（不带 /…/ 与 flag），后端按行匹配 */
  pattern: string;
}

/** GET /api/settings/chapter-rules 的响应；PUT 是全量替换。 */
export interface ChapterRules {
  rules: ChapterRule[];
}

/**
 * AI 审核开关（GET /api/settings/ai）：开关落库，key 只从环境变量进进程。
 * apiKeyConfigured 是派生值——只回答「配置里有没有 key」，永远不携带 key 本身。
 */
export interface AISettings {
  enabled: boolean;
  apiKeyConfigured: boolean;
}

// --- P3 契约（docs/specs/2026-10-25-p3-contract.md） ---

/**
 * 轻量书目：GET /api/books/random 与 GET /api/series/{name} 的 books 元素。
 * 后端按列表项同一套字段投影，前端直接复用书架卡片渲染，不再单独裁剪。
 */
export type BookBrief = BookSummary;

/** POST /api/books/batch：单事务逐本应用同一 patch；单本失败只进 failed，不整体回滚。 */
export interface BookBatchResult {
  updated: number[];
  failed: { id: number; message: string }[];
}

/** 自定义书单（扁平收藏夹，一本书可属多个书单）。 */
export interface Collection {
  id: number;
  name: string;
  bookCount: number;
}

/** 系列聚合项：只统计 series 非空的书，排序按最近更新时间倒序。 */
export interface SeriesSummary {
  name: string;
  bookCount: number;
  wordCount: number;
  /** 该系列内最近一章的入库时间；系列内没有章节时为空串 */
  lastUpdatedAt: string;
  /** 取封面用的书 id；0 表示没有可用封面 */
  coverBookId: number;
}

/** GET /api/series 的响应：items 之外只保证 total。 */
export interface SeriesPage {
  items: SeriesSummary[];
  total: number;
}

/** GET /api/series/{name}：系列不存在（或没有书）时后端 404。 */
export interface SeriesDetail {
  name: string;
  books: BookBrief[];
}

/** GET /api/stats/reading 的单日聚合：近 N 天每天一项，没有阅读的日期 seconds 为 0。 */
export interface DaySeconds {
  /** YYYY-MM-DD（本地库时区口径由后端统一） */
  date: string;
  seconds: number;
}

export interface ReadingStats {
  totalSeconds: number;
  todaySeconds: number;
  weekSeconds: number;
  /** 从今天往前连续有阅读记录的天数；今天没有记录即为 0。 */
  streak: number;
  days: DaySeconds[];
}

/** GET /api/reading/history 的一行：按书聚合（最后章节 / 最后时间 / 累计时长 / 阅读天数）。 */
export interface ReadingHistoryRow {
  bookId: number;
  bookTitle: string;
  chapterId: number;
  chapterTitle: string;
  lastReadAt: string;
  totalSeconds: number;
  days: number;
}

/** GET /api/reading/history 的响应：只保证 items/total，页码由请求方自己记。 */
export interface ReadingHistoryResult {
  items: ReadingHistoryRow[];
  total: number;
}

/**
 * 当前账号（GET /api/auth/me）。没有密码，所以没有邮箱 / 头像之类——只有名字有用。
 *
 * 前端**读不到会话 Cookie**（HttpOnly），"我是谁" 只能靠这个接口回答：
 * 200 即已登录（开放模式下后端直接回默认账号，行为与单用户时代一致），401 即未登录。
 */
export interface AuthUser {
  id: number;
  name: string;
  createdAt: string;
  /** 空串 = 建号后从未登录过 */
  lastSeenAt: string;
}

/**
 * 账号列表里的一行（GET /api/auth/users）：与 AuthUser 同构，只有「最后活跃」的表示法不同——
 * 列表接口用 null 表示建号后从未登录，me() 用空串。展示层把两者统一成「从未登录」。
 */
export type AuthUserSummary = Omit<AuthUser, "lastSeenAt"> & { lastSeenAt: string | null };

/** GET /api/auth/users 的响应：items 之外只保证 total（账号总数）。 */
export interface AuthUsersResult {
  items: AuthUserSummary[];
  total: number;
}

/**
 * 我的书架里的一行（GET /api/shelf）：书简介字段 + 加入时间。
 *
 * 后端取的是 BookBrief 列（比 BookSummary 少 volumeCount / intro / status 等），
 * 所以这里的简介字段是可选的；渲染前必须过 `shelfItemsToBooks` 补齐默认值，
 * 不要直接把 ShelfItem 当 BookSummary 用。
 */
export interface ShelfItem extends BookSummary {
  /** 加入书架的时间（UTC ISO-8601） */
  addedAt: string;
}

/** GET /api/shelf/ids：当前账号书架的全部书 id（不分页，用于列表打「已加入」标记）。 */
export interface ShelfIdsResponse {
  bookIds: number[];
}

/** POST /api/reading/sessions 的请求体：页面可见时累计，≥60s 且切章/隐藏/卸载才提交一次。 */
export interface ReadingSessionInput {
  bookId: number;
  chapterId: number;
  /** 1–86400，越界后端 400 */
  seconds: number;
  /** 缺省由后端取当前时间 */
  startedAt?: string;
}

// --- AI 审核（docs/api/api-reviews.md 是事实源） ---

/**
 * 审核记录状态机：pending 等 AI → ready 有结论 → 人工决策 → confirmed_*；
 * rolled_back 是「替换后又回滚」的终态。非法迁移后端一律 409 review_state。
 */
export type ReviewStatus =
  | "pending"
  | "ready"
  | "needs_manual"
  | "confirmed_replace"
  | "confirmed_keep"
  | "rolled_back";

/** 列表的 status 参数：all = 全部历史，其余是单状态。未知值后端按字面筛（空列表）。 */
export type ReviewStatusFilter = ReviewStatus | "all";

/** 一份质量分；computed=false 表示**没算过**（不是 0 分，0 分看起来像完美版本）。 */
export interface ReviewQuality {
  garbageRatio: number;
  noiseRatio: number;
  structureScore: number;
  duplicateRatio: number;
  chapters: number;
  chars: number;
  computed: boolean;
}

/** 审核里的两版书之一；supersededBy=0 表示这一版是**当前在用**的版本。书被删时只留 id。 */
export interface ReviewBook {
  id: number;
  title: string;
  chapterCount: number;
  wordCount: number;
  supersededBy: number;
  quality: ReviewQuality;
}

/** 规则侧一个维度的对比：key 是 garbage / noise / structure。 */
export interface ReviewDimension {
  key: string;
  old: number;
  new: number;
}

/** 规则侧质量对比；note 非空 = 走了保守路径（哪一侧没有质量分）。 */
export interface ReviewQualityCompare {
  old: ReviewQuality;
  new: ReviewQuality;
  better: ReviewDimension[];
  worse: ReviewDimension[];
  note?: string;
}

/** AI 结论；needs_manual 时 winner 为空，reasons 是「为什么没有结论」的原因。 */
export interface ReviewAI {
  winner?: "new" | "old" | "equal" | "";
  confidence: number;
  reasons: string[];
}

/** 一条升级审核记录（列表 items[] 与两个决策端点的响应同形）。 */
export interface ImportReview {
  id: number;
  jobId: number;
  status: ReviewStatus;
  hitRatio: number;
  oldBook: ReviewBook;
  newBook: ReviewBook;
  quality: ReviewQualityCompare;
  ai: ReviewAI;
  ruleReasons: string[];
  createdAt: string;
  decidedAt?: string | null;
  decidedBy: string;
}

/** GET /api/import/reviews 的响应：items 之外只有 AI 开关当前值。 */
export interface ImportReviews {
  items: ImportReview[];
  aiEnabled: boolean;
}

/** decide 的动作：replace 换成新版，keep 保留当前版本（书侧不动）。 */
export type ReviewDecision = "replace" | "keep";

// --- 推荐与埋点（docs/specs/2026-11-01-recommend-design.md §4 契约） ---

/**
 * 埋点事件类型：与 rec 库 events.kind 一一对应，也是后端 kind 白名单的全集
 * （设计文档 §2 的「有信号的事件」+ §4 的校验口径）。
 *
 * **没有 `start`**：§5 写的是「挂载记 start」，但 §2 / §4 的落库与校验白名单里都没有它，
 * 后端对不在白名单里的 kind 整批 400（批量上报是重试单位，混合接受会让客户端无法判断该重发哪条）。
 * 所以详情页只在离开时上报一条 `view`（value = 停留秒数）——它同样能表达「进来过、待了多久」，
 * 且不会把同一批里的其他事件一起带进 400。要不要加 `start` 由后端契约决定，前端不单方面发明。
 */
export type TrackEventKind =
  | "impression"
  | "click"
  | "view"
  | "read"
  | "search"
  | "shelf"
  | "rating"
  /** 发现页「不感兴趣」：负反馈，后端把这本书从后续推荐里压下去 */
  | "dislike";

/**
 * POST /api/events 的一条事件。字段都可缺省：埋点只带**已知**的维度，
 * 缺的维度由后端按 book_id 反查，不在这里猜。
 */
export interface TrackEvent {
  kind: TrackEventKind;
  bookId?: number;
  authorId?: number;
  categoryL1?: string;
  categoryL2?: string;
  /** 数值语义按 kind 定：view = 停留秒数，rating = 星级，其余缺省。 */
  value?: number;
  /** 自由文本（如搜索词）；超长后端 400，所以调用方只放短串。 */
  extra?: string;
  /** 客户端时间（ISO-8601）。当前不发送：服务端按接收时间落 ts，避免两台时钟不一致。 */
  ts?: string;
}

/** POST /api/events 的请求体：一次 ≤100 条（lib/telemetry 自己拆批）。 */
export interface TrackEventsBody {
  events: TrackEvent[];
}

/** POST /api/events 的响应：只回实际落库条数。 */
export interface TrackEventsResult {
  accepted: number;
}

/**
 * GET /api/recommend 的一条推荐。字段由后端投影，前端不猜：
 * `reason` 是给人看的一句话（「因为你常看武侠」），`score` 只用于排序与调试。
 */
export interface RecommendItem {
  bookId: number;
  title: string;
  /** 作者 id：发现页「不喜欢这个作者」要把它传给后端；缺省（0 / undefined）时那一项置灰 */
  authorId?: number;
  authorName: string;
  /** 空串 = 未分类（与 BookMeta 同口径） */
  categoryL1: string;
  /** 空串 = 该大分类下未细分 */
  categoryL2: string;
  hasCover: boolean;
  /** 体量信息：主库查询本来就有这几列，推荐卡直接显示，不为每张卡再查一次详情 */
  wordCount?: number;
  chapterCount?: number;
  /** 连载状态：''=未知 / ongoing / completed */
  serialStatus?: string;
  /** 推荐理由：直接展示，不由前端拼装 */
  reason: string;
  /**
   * 理由的稳定标识（如 `interest:武侠` / `hot` / `similar.author`）。
   * 埋点与看板分组用它而不是用文案：文案改了不该把历史统计拆成两行。
   * 旧服务端不带这个字段，前端回落到 `reason`。
   */
  reasonCode?: string;
  score: number;
}

/**
 * 推荐流的三个来源（GET /api/recommend?tab=…）：
 * - `for_you` 为你推荐：兴趣 + 共读 + 热门 + 新鲜 + 随机（设计 §3 的完整版）
 * - `resume` 继续阅读：读到一半、还没读完的书
 * - `random` 随机发现：均匀随机，不看兴趣（给「随便翻翻」用）
 */
export type RecommendTab = "for_you" | "resume" | "random";

/** 详情页「你可能也喜欢」：`similar=<bookId>` 走同一份响应形状（items 带 reason）。 */
export interface SimilarQuery {
  similar: number;
  limit?: number;
}

/** GET /api/recommend 的响应。`generatedAt` 用于「这一批是什么时候算的」。 */
export interface RecommendResult {
  items: RecommendItem[];
  generatedAt: string;
}

// --- 推荐看板（GET /api/recommend/stats，契约见 docs/api/api-recommend.md） ---

/** 窗口汇总：days 是实际统计天数，其余都是该窗口内的量。 */
export interface RecommendStatsSummary {
  days: number;
  impressions: number;
  clicks: number;
  reads: number;
  /** 阅读时长（秒）：前端换算成分钟展示 */
  readSeconds: number;
  shelfAdds: number;
}

/** 某一天的事件量。`date` 是 UTC 的 YYYY-MM-DD（与后端 daily 同口径）。 */
export interface RecommendStatsDay {
  date: string;
  impressions: number;
  clicks: number;
  reads: number;
  /** 当天阅读时长（秒） */
  seconds: number;
}

/**
 * 兴趣维度的五个分档：大分类 / 小分类 / 作者 / 标签 / 系列。
 *
 * 大分类与小分类分开，是因为它们的 key 空间重叠（「武侠」既有可能是大分类，
 * 也有可能是某个小分类的名字）。合在一个 dim 里时看板会出现「武侠 69.4 / 江湖 69.4」
 * 这种成对重复、看起来像算重了的行。
 *
 * 标签与系列是后端的另外两维：取值形态与分类一致（`name` 与 `key` 相同、没有
 * 「编号 → 名字」的翻译），所以前端只是多两组横条，不需要额外的名字映射。
 */
export type RecommendInterestDim =
  | "category_l1"
  | "category_l2"
  | "author"
  | "tag"
  | "series";

/**
 * 兴趣画像一行。
 *
 * - `dim`：五个维度之一（见 RecommendInterestDim）。老后端的 `category` 已不再出现，
 *   前端不认识的值一律不渲染（不猜、也不会让整张图报错）。
 * - `key`：分类名 / 标签名 / 系列名，或作者 id 的十进制文本（稳定标识，不随重命名变）；
 * - `name`：给人看的名字。分类、标签、系列的 name = key；作者是作者名，查不到时为空串。
 *   后端这个字段是后加的，所以类型上可选——缺省时前端退回 `key`，不猜名字。
 */
export interface RecommendInterest {
  dim: RecommendInterestDim;
  key: string;
  /** 展示名（作者名 / 分类名）；缺省或空串时用 key 兜底 */
  name?: string;
  /** 已按 14 天衰减的兴趣权重 */
  weight: number;
}

/** 热门书一行：三个计数都是统计窗口内的量。 */
export interface RecommendHotBook {
  bookId: number;
  title: string;
  authorName: string;
  reads: number;
  clicks: number;
  /** 这本书被读了多少秒（窗口内合计） */
  seconds: number;
}

/**
 * 推荐理由的点击分布（驾驶舱「点击理由分布」）。
 *
 * `impressions` 可选：从 task-9 起前端把理由也塞进 impression 的 extra，
 * 所以后端能按理由同时给出曝光与点击；只给 clicks 时图照常出（只是不显示点击率）。
 */
export interface RecommendReasonMix {
  /** 分组键：稳定 code（历史数据已被后端归一化成同一形态） */
  reason: string;
  /** 当前版本的展示文案；缺省时前端回落到 reason（旧服务端） */
  label?: string;
  impressions?: number;
  clicks: number;
}

/** 分类维度的点击分布（驾驶舱「分类点击分布」）：categoryL1 空串 = 未分类。 */
export interface RecommendCategoryClicks {
  categoryL1: string;
  clicks: number;
}

/** GET /api/recommend/stats：推荐库未启用时后端 503 unavailable。 */
export interface RecommendStats {
  summary: RecommendStatsSummary;
  daily: RecommendStatsDay[];
  interests: RecommendInterest[];
  hotBooks: RecommendHotBook[];
  /** 点击理由分布（可选：老后端没有时这张图走空态，不影响其它图） */
  reasonMix?: RecommendReasonMix[];
  /** 分类点击分布（可选，同上） */
  categoryClicks?: RecommendCategoryClicks[];
}

/** 一个版本的四类条目（GET /api/releases）。空分组是 []，不是 null。 */
export interface ReleaseSections {
  added: string[];
  changed: string[];
  fixed: string[];
  upgrade: string[];
}

/** 一个版本的发版说明。retro = 事后从 git 历史回溯整理（早期版本发版时没写）。 */
export interface ReleaseEntry {
  version: string;
  /** YYYY-MM-DD；缺省为空串 */
  date: string;
  retro: boolean;
  sections: ReleaseSections;
}

/**
 * GET /api/releases：内置发版说明，按版本倒序。
 *
 * current 是当前运行版本在说明里的对应条目；版本对不上（如开发构建）时为空串，
 * 页面据此决定是否高亮「正在使用」。
 */
export interface ReleaseNotes {
  current: string;
  items: ReleaseEntry[];
}

/** GET /api/version */
export interface VersionInfo {
  version: string;
  commit: string;
  builtAt: string;
}

/** 一条路由的耗时聚合（GET /api/metrics 的 http[] 项，按路由模板聚合）。 */
export interface RouteTiming {
  route: string;
  count: number;
  errors: number;
  clientErrors: number;
  avgMs: number;
  maxMs: number;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  /** 直方图各桶计数，长度 = 边界数 + 1 */
  buckets: number[];
}

/** 一条慢请求（GET /api/metrics 的 slowRequests[] 项，最新在前）。 */
export interface SlowRequest {
  at: string;
  route: string;
  method: string;
  path: string;
  status: number;
  durMs: number;
  sqlCount: number;
  sqlMs: number;
  requestId?: string;
}

/** 一个 SQL 模板的聚合（sql[] 项）。 */
export interface QueryTiming {
  query: string;
  count: number;
  errors: number;
  slowCount: number;
  avgMs: number;
  maxMs: number;
  p95Ms: number;
}

/** 一个连接池的 sql.DBStats 快照。 */
export interface PoolStat {
  name: string;
  open: number;
  inUse: number;
  idle: number;
  waitCount: number;
  waitMs: number;
}

/** 进程运行时快照。 */
export interface RuntimeStat {
  goVersion: string;
  goroutines: number;
  heapMB: number;
  numGC: number;
  pools: PoolStat[];
}

/** GET /api/metrics：进程内（重启清零）的运行时性能快照。 */
export interface MetricsSnapshot {
  startedAt: string;
  uptimeSec: number;
  http: RouteTiming[];
  slowRequests: SlowRequest[];
  sql: QueryTiming[];
  runtime: RuntimeStat;
}
