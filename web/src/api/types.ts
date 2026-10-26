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

export interface SearchHit {
  bookId: number;
  bookTitle: string;
  chapterId: number;
  chapterIdx: number;
  chapterTitle: string;
  /**
   * 章节命中片段。检索范围收敛到书名 / 作者 / 章节标题后，后端**仍返回该字段，但恒为空字符串**；
   * 字段保留是为了契约稳定（调用方不必判 undefined）。渲染前先过 lib/snippet 的 hasSnippet。
   */
  snippet: string;
  score: number;
}

export interface SearchResult {
  q: string;
  /** 书名与作者命中（检索范围之一） */
  books: Page<BookSummary>;
  /** 章节标题命中（检索范围之二）；每项都带 snippet 字段但恒为空串 */
  chapters: Page<SearchHit>;
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
}

export interface DryRunSample {
  source: string;
  title: string;
  chapters: number;
  words: number;
  duplicate: boolean;
  reason: string;
}

/** 词表规则：命中关键词就给书打这个标签（导入时执行，只增不删，不覆盖手动标签）。 */
export interface TagRule {
  tag: string;
  keywords: string[];
}

export interface TagRules {
  rules: TagRule[];
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

/** POST /api/reading/sessions 的请求体：页面可见时累计，≥60s 且切章/隐藏/卸载才提交一次。 */
export interface ReadingSessionInput {
  bookId: number;
  chapterId: number;
  /** 1–86400，越界后端 400 */
  seconds: number;
  /** 缺省由后端取当前时间 */
  startedAt?: string;
}