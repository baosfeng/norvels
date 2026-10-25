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
  /** 本书书签总数；缺省时按「还不知道」处理，不展示。 */
  bookmarkCount?: number;
  importedAt: string;
  updatedAt: string;
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

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
  /** 连载状态；后端未升级时缺省，前端一律按 `""`（未知）处理。 */
  serialStatus?: SerialStatus;
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